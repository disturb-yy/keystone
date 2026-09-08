package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

var v2CommandName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// V2Command 是 Manifest V2 中不经 shell 解释的验证命令。
type V2Command struct {
	Name           string
	Argv           []string
	TimeoutSeconds int
}

// ProjectManifestV2 是 Verify/Commit 读取的严格项目配置。
type ProjectManifestV2 struct {
	Version        int
	ProjectID      domain.ProjectID
	VerifyCommands []V2Command
	CommitTemplate string
}

// Validate 检查 V2 的结构、命令边界和受控 Commit 模板。
func (m ProjectManifestV2) Validate() error {
	if m.Version != 2 {
		return fmt.Errorf("%w: manifest version must be 2", domain.ErrManifestInvalid)
	}
	if err := m.ProjectID.Validate(); err != nil {
		return err
	}
	if len(m.VerifyCommands) < 1 || len(m.VerifyCommands) > 32 {
		return fmt.Errorf("%w: verify.commands count is invalid", domain.ErrManifestInvalid)
	}
	seen := make(map[string]struct{}, len(m.VerifyCommands))
	for _, command := range m.VerifyCommands {
		if !v2CommandName.MatchString(command.Name) || len(command.Argv) < 1 || len(command.Argv) > 64 || command.TimeoutSeconds < 1 || command.TimeoutSeconds > 1800 {
			return fmt.Errorf("%w: verify command envelope is invalid", domain.ErrManifestInvalid)
		}
		if _, exists := seen[command.Name]; exists {
			return fmt.Errorf("%w: duplicate verify command name", domain.ErrManifestInvalid)
		}
		seen[command.Name] = struct{}{}
		argvBytes := 0
		for _, arg := range command.Argv {
			if arg == "" || !utf8.ValidString(arg) || len([]byte(arg)) > 4<<10 {
				return fmt.Errorf("%w: verify command argument is invalid", domain.ErrManifestInvalid)
			}
			argvBytes += len([]byte(arg))
		}
		if argvBytes > 16<<10 {
			return fmt.Errorf("%w: verify command arguments are too large", domain.ErrManifestInvalid)
		}
	}
	if len([]byte(m.CommitTemplate)) > 4<<10 || !utf8.ValidString(m.CommitTemplate) || strings.IndexByte(m.CommitTemplate, 0) >= 0 {
		return fmt.Errorf("%w: commit.template is invalid", domain.ErrManifestInvalid)
	}
	remaining := m.CommitTemplate
	for _, marker := range []string{"{change_id}", "{ticket_id}", "{ticket_title}"} {
		remaining = strings.ReplaceAll(remaining, marker, "")
	}
	if strings.ContainsAny(remaining, "{}") || strings.Contains(remaining, "Keystone-Change-ID:") || strings.Contains(remaining, "Keystone-Ticket-ID:") || strings.Contains(remaining, "Keystone-Commit-ID:") {
		return fmt.Errorf("%w: commit.template contains an unsupported value", domain.ErrManifestInvalid)
	}
	return nil
}

// Digests 返回规范化命令和模板的语义摘要。
func (m ProjectManifestV2) Digests() (string, string, error) {
	commands, err := json.Marshal(m.VerifyCommands)
	if err != nil {
		return "", "", err
	}
	template, err := json.Marshal(struct {
		Template string `json:"template"`
	}{m.CommitTemplate})
	if err != nil {
		return "", "", err
	}
	commandDigest := sha256.Sum256(commands)
	templateDigest := sha256.Sum256(template)
	return hex.EncodeToString(commandDigest[:]), hex.EncodeToString(templateDigest[:]), nil
}

// ParseV2 严格解析 M8 允许的单文档 YAML 子集。
//
// M8 的配置形状是有界的；这里使用专用 parser 保持 Manifest adapter 的纯 Go
// 依赖和严格字段控制，不把通用 YAML 的 alias、tag 或隐式类型带入策略边界。
func ParseV2(data []byte) (ProjectManifestV2, error) {
	if len(data) == 0 || !utf8.Valid(data) {
		return ProjectManifestV2{}, invalidV2()
	}
	lines, err := v2Lines(string(data))
	if err != nil {
		return ProjectManifestV2{}, err
	}
	if len(lines) == 0 {
		return ProjectManifestV2{}, invalidV2()
	}
	manifest := ProjectManifestV2{}
	seenRoot := make(map[string]struct{})
	for index := 0; index < len(lines); {
		line := lines[index]
		if line.indent != 0 {
			return ProjectManifestV2{}, invalidV2()
		}
		key, value, err := splitV2Key(line.text)
		if err != nil {
			return ProjectManifestV2{}, err
		}
		if _, exists := seenRoot[key]; exists {
			return ProjectManifestV2{}, invalidV2()
		}
		seenRoot[key] = struct{}{}
		switch key {
		case "version":
			if value == "" {
				return ProjectManifestV2{}, invalidV2()
			}
			manifest.Version, err = strconv.Atoi(value)
			if err != nil {
				return ProjectManifestV2{}, invalidV2()
			}
			index++
		case "project_id":
			manifest.ProjectID = domain.ProjectID(value)
			index++
		case "verify":
			if value != "" {
				return ProjectManifestV2{}, invalidV2()
			}
			index++
			index, err = parseVerifyBlock(lines, index, &manifest)
			if err != nil {
				return ProjectManifestV2{}, err
			}
		case "commit":
			if value != "" {
				return ProjectManifestV2{}, invalidV2()
			}
			index++
			index, err = parseCommitBlock(lines, index, &manifest)
			if err != nil {
				return ProjectManifestV2{}, err
			}
		default:
			return ProjectManifestV2{}, invalidV2()
		}
	}
	if _, ok := seenRoot["version"]; !ok {
		return ProjectManifestV2{}, invalidV2()
	}
	if _, ok := seenRoot["project_id"]; !ok {
		return ProjectManifestV2{}, invalidV2()
	}
	if _, ok := seenRoot["verify"]; !ok {
		return ProjectManifestV2{}, invalidV2()
	}
	if err := manifest.Validate(); err != nil {
		return ProjectManifestV2{}, err
	}
	return manifest, nil
}

// Parse 是 V2 parser 的稳定公开入口。
func Parse(data []byte) (ProjectManifestV2, error) { return ParseV2(data) }

type v2Line struct {
	indent int
	text   string
}

func v2Lines(text string) ([]v2Line, error) {
	if strings.Contains(text, "\r") {
		text = strings.ReplaceAll(text, "\r\n", "\n")
		if strings.Contains(text, "\r") {
			return nil, invalidV2()
		}
	}
	var result []v2Line
	for _, raw := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if trimmed == "---" || trimmed == "..." || strings.HasPrefix(trimmed, "&") || strings.HasPrefix(trimmed, "*") || strings.Contains(trimmed, " !") || strings.HasPrefix(trimmed, "!") {
			return nil, invalidV2()
		}
		withoutComment, err := stripV2Comment(raw)
		if err != nil {
			return nil, err
		}
		withoutComment = strings.TrimRight(withoutComment, " \t")
		if strings.TrimSpace(withoutComment) == "" {
			continue
		}
		leading := len(withoutComment) - len(strings.TrimLeft(withoutComment, " \t"))
		if strings.ContainsRune(withoutComment[:leading], '\t') {
			return nil, invalidV2()
		}
		indent := len(withoutComment) - len(strings.TrimLeft(withoutComment, " "))
		if indent%2 != 0 {
			return nil, invalidV2()
		}
		result = append(result, v2Line{indent: indent, text: strings.TrimSpace(withoutComment)})
	}
	return result, nil
}

func parseVerifyBlock(lines []v2Line, index int, manifest *ProjectManifestV2) (int, error) {
	if index >= len(lines) || lines[index].indent != 2 || lines[index].text != "commands:" {
		return index, invalidV2()
	}
	index++
	if index >= len(lines) || lines[index].indent != 4 || !strings.HasPrefix(lines[index].text, "-") {
		return index, invalidV2()
	}
	for index < len(lines) && lines[index].indent == 4 && strings.HasPrefix(lines[index].text, "-") {
		command := V2Command{}
		item := strings.TrimSpace(strings.TrimPrefix(lines[index].text, "-"))
		if item != "" {
			key, value, err := splitV2Key(item)
			if err != nil || key != "name" {
				return index, invalidV2()
			}
			command.Name = value
		}
		index++
		seen := map[string]bool{}
		if command.Name != "" {
			seen["name"] = true
		}
		for index < len(lines) && lines[index].indent == 6 {
			key, value, err := splitV2Key(lines[index].text)
			if err != nil || seen[key] {
				return index, invalidV2()
			}
			seen[key] = true
			switch key {
			case "name":
				command.Name = value
			case "argv":
				command.Argv, err = parseV2Array(value)
				if err != nil {
					return index, err
				}
			case "timeout_seconds":
				command.TimeoutSeconds, err = strconv.Atoi(value)
				if err != nil {
					return index, invalidV2()
				}
			default:
				return index, invalidV2()
			}
			index++
		}
		if !seen["name"] || !seen["argv"] || !seen["timeout_seconds"] {
			return index, invalidV2()
		}
		manifest.VerifyCommands = append(manifest.VerifyCommands, command)
	}
	if len(manifest.VerifyCommands) == 0 {
		return index, invalidV2()
	}
	return index, nil
}

func parseCommitBlock(lines []v2Line, index int, manifest *ProjectManifestV2) (int, error) {
	if index >= len(lines) || lines[index].indent != 2 {
		return index, invalidV2()
	}
	seen := false
	for index < len(lines) && lines[index].indent == 2 {
		key, value, err := splitV2Key(lines[index].text)
		if err != nil || key != "template" || seen {
			return index, invalidV2()
		}
		seen = true
		manifest.CommitTemplate = value
		index++
	}
	return index, nil
}

func splitV2Key(text string) (string, string, error) {
	colon := strings.IndexByte(text, ':')
	if colon <= 0 || strings.TrimSpace(text[:colon]) != text[:colon] {
		return "", "", invalidV2()
	}
	key := text[:colon]
	value, err := parseV2Scalar(strings.TrimSpace(text[colon+1:]))
	if err != nil {
		return "", "", err
	}
	return key, value, nil
}

func parseV2Array(value string) ([]string, error) {
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil, invalidV2()
	}
	inner := strings.TrimSpace(value[1 : len(value)-1])
	if inner == "" {
		return nil, invalidV2()
	}
	parts := splitV2CSV(inner)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item, err := parseV2Scalar(strings.TrimSpace(part))
		if err != nil || item == "" {
			return nil, invalidV2()
		}
		result = append(result, item)
	}
	return result, nil
}

func splitV2CSV(value string) []string {
	var result []string
	start := 0
	var quote byte
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '\'', '"':
			if quote == 0 {
				quote = value[index]
			} else if quote == value[index] && (index == 0 || value[index-1] != '\\') {
				quote = 0
			}
		case ',':
			if quote == 0 {
				result = append(result, value[start:index])
				start = index + 1
			}
		}
	}
	result = append(result, value[start:])
	return result
}

func parseV2Scalar(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if value[0] == '"' {
		if len(value) < 2 || value[len(value)-1] != '"' {
			return "", invalidV2()
		}
		var parsed string
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			return "", invalidV2()
		}
		return parsed, nil
	}
	if value[0] == '\'' {
		if len(value) < 2 || value[len(value)-1] != '\'' {
			return "", invalidV2()
		}
		return strings.ReplaceAll(value[1:len(value)-1], "''", "'"), nil
	}
	if strings.ContainsAny(value, "&*!<>") || strings.Contains(value, "{") || strings.Contains(value, "}") {
		return "", invalidV2()
	}
	return value, nil
}

func stripV2Comment(value string) (string, error) {
	var quote byte
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '\'', '"':
			if quote == 0 {
				quote = value[index]
			} else if quote == value[index] && (index == 0 || value[index-1] != '\\') {
				quote = 0
			}
		case '#':
			if quote == 0 && (index == 0 || value[index-1] == ' ' || value[index-1] == '\t') {
				return value[:index], nil
			}
		}
	}
	if quote != 0 {
		return "", invalidV2()
	}
	return value, nil
}

func invalidV2() error { return fmt.Errorf("parse project manifest v2: %w", domain.ErrManifestInvalid) }
