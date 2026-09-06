// Package execution 定义 Worker 到本机 Runtime 的执行端口和观察结果。
package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	// RuntimeCodex 是 V1 支持的 Runtime capability。
	RuntimeCodex = "codex"

	ArtifactStdout       ArtifactKind = "stdout"
	ArtifactStderr       ArtifactKind = "stderr"
	ArtifactDiff         ArtifactKind = "diff"
	ArtifactChangedFiles ArtifactKind = "changed_files"
)

// ArtifactKind 是四类可传输执行证据的固定名称。
type ArtifactKind string

// RuntimeAdapter 将受限执行输入转换为 Runtime 的观察事实。
type RuntimeAdapter interface {
	Name() string
	Run(context.Context, ExecutionInput) (RuntimeResult, error)
}

// ExecutionInput 是 Daemon 已授权并由 Worker 校验后的 Runtime 输入。
type ExecutionInput struct {
	Workspace      string
	Instruction    string
	Runtime        string
	BeforeRevision string
	Timeout        time.Duration
	Environment    []string
	Limits         Limits
}

// Validate 检查不会改变业务状态的本地执行前置条件。
func (in ExecutionInput) Validate() error {
	if strings.TrimSpace(in.Workspace) == "" || strings.TrimSpace(in.Instruction) == "" {
		return fmt.Errorf("validate execution input: workspace and instruction are required")
	}
	if in.Runtime == "" {
		in.Runtime = RuntimeCodex
	}
	if in.Timeout < 0 {
		return fmt.Errorf("validate execution input: timeout must not be negative")
	}
	return nil
}

// RuntimeResult 保存进程和 Workspace 的独立观察结果，不表达 AgentRun 权威状态。
type RuntimeResult struct {
	StartedAt       time.Time
	CompletedAt     time.Time
	ExitCode        *int
	TimedOut        bool
	BeforeRevision  string
	AfterRevision   string
	Stdout          Artifact
	Stderr          Artifact
	Diff            Artifact
	ChangedFiles    Artifact
	ChangedFileList []string
	GuardFindings   []string
	CaptureFailures []CaptureFailure
	FailureReason   string
}

// Artifact 是可传输的有界证据内容及其身份摘要。
type Artifact struct {
	Kind         ArtifactKind
	Content      []byte
	SHA256       string
	SizeBytes    int64
	Truncated    bool
	CaptureError string
}

// NewArtifact 根据实际保存的内容创建摘要；调用方仍需保留截断事实。
func NewArtifact(kind ArtifactKind, content []byte, truncated bool, captureErr error) Artifact {
	digest := sha256.Sum256(content)
	artifact := Artifact{
		Kind:      kind,
		Content:   append([]byte(nil), content...),
		SHA256:    hex.EncodeToString(digest[:]),
		SizeBytes: int64(len(content)),
		Truncated: truncated,
	}
	if captureErr != nil {
		artifact.CaptureError = captureErr.Error()
	}
	return artifact
}

// Validate 检查 Artifact 的摘要、长度和 kind 表达。
func (a Artifact) Validate() error {
	if a.Kind != ArtifactStdout && a.Kind != ArtifactStderr && a.Kind != ArtifactDiff && a.Kind != ArtifactChangedFiles {
		return fmt.Errorf("validate artifact: unsupported kind %q", a.Kind)
	}
	if a.SizeBytes != int64(len(a.Content)) || len(a.SHA256) != sha256.Size*2 {
		return fmt.Errorf("validate artifact %s: size or digest mismatch", a.Kind)
	}
	digest := sha256.Sum256(a.Content)
	if hex.EncodeToString(digest[:]) != a.SHA256 {
		return fmt.Errorf("validate artifact %s: digest mismatch", a.Kind)
	}
	return nil
}

// CaptureFailure 描述独立证据采集阶段的失败，而不是 Runtime 的自报文本。
type CaptureFailure struct {
	Kind  ArtifactKind
	Stage string
	Error string
}

// Limits 是单项和单次 Report 的有界采集限制。
type Limits struct {
	StdoutBytes       int64
	StderrBytes       int64
	DiffBytes         int64
	ChangedFilesBytes int64
	TotalBytes        int64
}

// DefaultLimits 返回 Ticket 06 的默认证据上限。
func DefaultLimits() Limits {
	return Limits{
		StdoutBytes:       16 << 20,
		StderrBytes:       16 << 20,
		DiffBytes:         16 << 20,
		ChangedFilesBytes: 1 << 20,
		TotalBytes:        64 << 20,
	}
}
