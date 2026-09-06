package execution

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
)

type boundedBuffer struct {
	bytes.Buffer
	limit     int64
	truncated bool
}

func newBoundedBuffer(limit int64) *boundedBuffer {
	return &boundedBuffer{limit: limit}
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	if b.limit < 0 {
		return len(value), nil
	}
	remaining := b.limit - int64(b.Len())
	if remaining <= 0 {
		b.truncated = len(value) > 0
		return len(value), nil
	}
	if int64(len(value)) > remaining {
		_, _ = b.Buffer.Write(value[:remaining])
		b.truncated = true
		return len(value), nil
	}
	_, _ = b.Buffer.Write(value)
	return len(value), nil
}

func (b *boundedBuffer) artifact(kind ArtifactKind) Artifact {
	return NewArtifact(kind, b.Bytes(), b.truncated, nil)
}

// NormalizeChangedFiles 将 Git 观察到的路径变为稳定的 Workspace 相对集合。
func NormalizeChangedFiles(files []string) ([]string, error) {
	seen := make(map[string]struct{}, len(files))
	normalized := make([]string, 0, len(files))
	for _, file := range files {
		file = strings.TrimSpace(strings.ReplaceAll(file, "\\", "/"))
		if file == "" || strings.HasPrefix(file, "/") || file == "." || strings.HasPrefix(file, "../") || strings.Contains(file, "/../") {
			return nil, fmt.Errorf("normalize changed files: path %q is not workspace-relative", file)
		}
		file = strings.TrimPrefix(file, "./")
		if file == "" || strings.HasPrefix(file, "../") {
			return nil, fmt.Errorf("normalize changed files: path %q is not workspace-relative", file)
		}
		if _, ok := seen[file]; ok {
			continue
		}
		seen[file] = struct{}{}
		normalized = append(normalized, file)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func changedFilesContent(files []string, limit int64) Artifact {
	content := []byte(strings.Join(files, "\n"))
	buffer := newBoundedBuffer(limit)
	_, _ = buffer.Write(content)
	return buffer.artifact(ArtifactChangedFiles)
}

// ChangedFilesArtifact 编码规范化后的相对路径并应用单项上限。
func ChangedFilesArtifact(files []string, limit int64) Artifact {
	return changedFilesContent(files, limit)
}

func artifactLimit(kind ArtifactKind, limits Limits) int64 {
	switch kind {
	case ArtifactStdout:
		return limits.StdoutBytes
	case ArtifactStderr:
		return limits.StderrBytes
	case ArtifactDiff:
		return limits.DiffBytes
	case ArtifactChangedFiles:
		return limits.ChangedFilesBytes
	default:
		return 0
	}
}

func enforceTotalLimit(result *RuntimeResult, limits Limits) {
	if limits.TotalBytes <= 0 {
		return
	}
	artifacts := []*Artifact{&result.Stdout, &result.Stderr, &result.Diff, &result.ChangedFiles}
	var total int64
	for _, artifact := range artifacts {
		if total+artifact.SizeBytes <= limits.TotalBytes {
			total += artifact.SizeBytes
			continue
		}
		remaining := limits.TotalBytes - total
		if remaining < 0 {
			remaining = 0
		}
		if int64(len(artifact.Content)) > remaining {
			artifact.Content = append([]byte(nil), artifact.Content[:remaining]...)
			artifact.Truncated = true
			updated := NewArtifact(artifact.Kind, artifact.Content, true, nil)
			*artifact = updated
		}
		total += artifact.SizeBytes
	}
}

// EnforceTotalLimit 应用一次 Report 的总证据上限。
func EnforceTotalLimit(result *RuntimeResult, limits Limits) {
	enforceTotalLimit(result, limits)
}

// LimitBytes 保留有界前缀并返回是否发生截断。
func LimitBytes(value []byte, limit int64) ([]byte, bool) {
	if limit <= 0 || int64(len(value)) <= limit {
		return append([]byte(nil), value...), false
	}
	return append([]byte(nil), value[:limit]...), true
}

// SanitizeEnvironment 删除协议凭据、数据库连接和常见秘密变量。
// base 为空时读取当前进程环境；调用方不能把 Worker 的进程 secret 放入 base。
func SanitizeEnvironment(base []string) []string {
	if base == nil {
		base = os.Environ()
	}
	filtered := make([]string, 0, len(base))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || forbiddenEnvironmentName(name) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func forbiddenEnvironmentName(name string) bool {
	upper := strings.ToUpper(name)
	if upper == "PATH" || upper == "HOME" || upper == "TMPDIR" || upper == "TEMP" || upper == "TMP" {
		return false
	}
	for _, marker := range []string{"WORKER_SECRET", "LEASE_TOKEN", "KEYSTONE_DB", "DATABASE_URL", "DATABASE_DSN", "OPENAI_API_KEY", "CODEX_API_KEY", "PASSWORD", "SECRET", "TOKEN", "_DSN"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}
