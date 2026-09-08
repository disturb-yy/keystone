// Package domain 定义 Governance 的纯业务模型。
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	VerificationKind        = "verify"
	VerificationCapability  = "verification-v1"
	VerificationPending     = "pending"
	VerificationRunning     = "running"
	VerificationPass        = "pass"
	VerificationFail        = "fail"
	VerificationHuman       = "human_required"
	VerificationUnavailable = "unavailable"
	CriterionPass           = "pass"
	CriterionFail           = "fail"
	CriterionHuman          = "human_required"
	CriterionNotRun         = "not_run"
	CommandPassed           = "passed"
	CommandFailed           = "failed"
	CommandTimedOut         = "timed_out"
	CommandNotRun           = "not_run"
	CommitIntentPending     = "pending"
	CommitIntentCommitted   = "committed"
	CommitIntentHuman       = "human_required"
	MaxReviewInputBytes     = 64 << 10
	MaxVerificationOutput   = 256 << 10
)

var commandNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

var (
	ErrInvalidPolicy       = errors.New("invalid verification policy")
	ErrInvalidVerification = errors.New("invalid verification result")
	ErrInvalidCommit       = errors.New("invalid commit intent")
)

// VerificationCommand 是来自 BaseRevision Manifest 的固定命令。
type VerificationCommand struct {
	Name           string
	Argv           []string
	TimeoutSeconds int
}

// Validate 检查不经 shell 解释的参数数组和固定 timeout 边界。
func (c VerificationCommand) Validate() error {
	if !commandNamePattern.MatchString(c.Name) || len(c.Argv) < 1 || len(c.Argv) > 64 || c.TimeoutSeconds < 1 || c.TimeoutSeconds > 1800 {
		return fmt.Errorf("%w: command envelope is invalid", ErrInvalidPolicy)
	}
	bytes := 0
	for _, arg := range c.Argv {
		if arg == "" || !utf8.ValidString(arg) || len([]byte(arg)) > 4<<10 {
			return fmt.Errorf("%w: command argument is invalid", ErrInvalidPolicy)
		}
		bytes += len([]byte(arg))
	}
	if bytes > 16<<10 {
		return fmt.Errorf("%w: command arguments are too large", ErrInvalidPolicy)
	}
	return nil
}

// PolicySnapshot 是 Execute 接受时从 BaseRevision 固定的验证策略。
type PolicySnapshot struct {
	ID                   string
	ProjectID            string
	ChangeID             string
	BaseRevision         string
	VerificationDigest   string
	CommitTemplateDigest string
	Commands             []VerificationCommand
	CommitTemplate       string
	CreatedAt            time.Time
}

// Validate 确保策略快照包含可复算的规范化值和摘要。
func (p PolicySnapshot) Validate() error {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.ProjectID) == "" || strings.TrimSpace(p.ChangeID) == "" || strings.TrimSpace(p.BaseRevision) == "" || len(p.Commands) < 1 || len(p.Commands) > 32 || p.CreatedAt.IsZero() {
		return fmt.Errorf("%w: snapshot identity is invalid", ErrInvalidPolicy)
	}
	seen := make(map[string]struct{}, len(p.Commands))
	for _, command := range p.Commands {
		if err := command.Validate(); err != nil {
			return err
		}
		if _, exists := seen[command.Name]; exists {
			return fmt.Errorf("%w: duplicate command name", ErrInvalidPolicy)
		}
		seen[command.Name] = struct{}{}
	}
	if err := ValidateCommitTemplate(p.CommitTemplate); err != nil {
		return err
	}
	verificationDigest, templateDigest, err := PolicyDigests(p.Commands, p.CommitTemplate)
	if err != nil || p.VerificationDigest != verificationDigest || p.CommitTemplateDigest != templateDigest {
		return fmt.Errorf("%w: policy digest does not match normalized values", ErrInvalidPolicy)
	}
	return nil
}

// ValidateCommitTemplate 检查受控模板占位符和 trailer 注入边界。
func ValidateCommitTemplate(template string) error {
	if len([]byte(template)) > 4<<10 || !utf8.ValidString(template) || strings.IndexByte(template, 0) >= 0 {
		return fmt.Errorf("%w: commit template is invalid", ErrInvalidPolicy)
	}
	for _, marker := range []string{"{change_id}", "{ticket_id}", "{ticket_title}"} {
		template = strings.ReplaceAll(template, marker, "")
	}
	if strings.Contains(template, "{") || strings.Contains(template, "}") || strings.Contains(template, "Keystone-Change-ID:") || strings.Contains(template, "Keystone-Ticket-ID:") || strings.Contains(template, "Keystone-Commit-ID:") {
		return fmt.Errorf("%w: commit template contains an unsupported placeholder or trailer", ErrInvalidPolicy)
	}
	return nil
}

// PolicyDigests 以字段顺序稳定的 JSON 形成语义摘要。
func PolicyDigests(commands []VerificationCommand, template string) (string, string, error) {
	commandJSON, err := json.Marshal(commands)
	if err != nil {
		return "", "", err
	}
	templateJSON, err := json.Marshal(struct {
		Template string `json:"template"`
	}{Template: template})
	if err != nil {
		return "", "", err
	}
	commandDigest := sha256.Sum256(commandJSON)
	templateDigest := sha256.Sum256(templateJSON)
	return hex.EncodeToString(commandDigest[:]), hex.EncodeToString(templateDigest[:]), nil
}

// VerificationIntent 是 Verify/FinalVerify 在等待 Worker 前落盘的不可变请求身份。
type VerificationIntent struct {
	ID                    string
	ChangeID              string
	TicketID              string
	PolicySnapshotID      string
	PolicyDigest          string
	InputRevision         string
	CandidateTreeIdentity string
	Final                 bool
	Status                string
	Attempt               int
	CreatedAt             time.Time
}

// VerificationCommandResult 是一个固定命令的受限结果摘要。
type VerificationCommandResult struct {
	Ordinal         int
	Name            string
	Status          string
	ExitCode        *int
	StdoutSHA256    string
	StdoutBytes     int64
	StdoutTruncated bool
	StderrSHA256    string
	StderrBytes     int64
	StderrTruncated bool
}

// AcceptanceCriterionResult 是一条 Canonical criterion 的独立审查结果。
type AcceptanceCriterionResult struct {
	TicketID    string
	Ordinal     int
	TextSHA256  string
	Outcome     string
	EvidenceIDs []string
}

// VerificationReport 是 Daemon 核验后的结构化 Verifier 结果。
type VerificationReport struct {
	IntentID              string
	Outcome               string
	Commands              []VerificationCommandResult
	Criteria              []AcceptanceCriterionResult
	ReviewSummary         string
	CandidateTreeIdentity string
	AfterRevision         string
	GuardFindings         []string
}

// CommitIntent 是 Git 写入前的唯一恢复锚点。
type CommitIntent struct {
	ID                     string
	ChangeID               string
	TicketID               string
	ExpectedParent         string
	CandidateTreeIdentity  string
	VerificationEvidenceID string
	CommitTemplateDigest   string
	KeystoneCommitID       string
	Status                 string
	CreatedAt              time.Time
}

// KeystoneCommit 是 SQLite 对已核验 Git commit 的权威关联。
type KeystoneCommit struct {
	ID             string
	ChangeID       string
	TicketID       string
	ParentRevision string
	AfterRevision  string
	TreeIdentity   string
	GitOID         string
	Message        string
	CreatedAt      time.Time
}

// CandidateRevision 表示 FinalVerify PASS 后允许 Integrate 的最终 revision。
type CandidateRevision struct {
	ChangeID     string
	Revision     string
	TreeIdentity string
	EvidenceID   string
	CreatedAt    time.Time
}

// RenderCommitMessage 将固定模板渲染为不含 Keystone trailer 的 subject/body。
func RenderCommitMessage(template, changeID, ticketID, ticketTitle string) (string, error) {
	if err := ValidateCommitTemplate(template); err != nil {
		return "", err
	}
	if template == "" {
		template = ticketTitle
	}
	message := strings.NewReplacer(
		"{change_id}", changeID,
		"{ticket_id}", ticketID,
		"{ticket_title}", ticketTitle,
	).Replace(template)
	if len([]byte(message)) > 8<<10 || strings.IndexByte(message, 0) >= 0 || !utf8.ValidString(message) {
		return "", fmt.Errorf("%w: rendered commit message is invalid", ErrInvalidCommit)
	}
	return strings.TrimSpace(message), nil
}

// TextDigest 返回 Canonical criterion 文本的 SHA-256。
func TextDigest(text string) string {
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])
}
