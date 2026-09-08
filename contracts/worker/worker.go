// Package worker 定义 Keystone Daemon 与独立 Worker 之间的窄传输边界。
package worker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// ProtocolVersionV1 是 Ticket 06 Worker Protocol 版本。
	ProtocolVersionV1 = "v1"
	// MaxBodyBytes 限制单个 Worker Protocol JSON body 的大小。
	MaxBodyBytes = 64 << 20

	// ResultModePlanningCandidate 要求 Runtime 单独返回可由 Daemon 校验的候选结果。
	ResultModePlanningCandidate = "planning_candidate"
)

// Outcome 表示 Worker 对一次 AgentRun 执行结果的传输值。
//
// Outcome 使用不透明且可扩展的字符串表示，具体解释由后续协议决定；
// 本包只固定其 JSON 字符串编码，不定义生命周期状态机或状态推进语义。
type Outcome string

// Register 表示 Worker 注册时发送的最小传输信息。
type Register struct {
	// WorkerID 是 Worker 在该传输边界上的标识。
	WorkerID string `json:"worker_id"`

	// ProtocolVersion 是 Worker 支持的 Worker Protocol 版本标识。
	ProtocolVersion string `json:"protocol_version"`

	// Capabilities 列出 Worker 声明的能力标识。
	Capabilities []string `json:"capabilities"`
}

// RegisterResponse 是 Daemon 接受注册后返回的非敏感会话信息。
type RegisterResponse struct {
	WorkerID              string   `json:"worker_id"`
	ProtocolVersion       string   `json:"protocol_version"`
	HeartbeatIntervalSecs int      `json:"heartbeat_interval_seconds"`
	LeaseTTLSeconds       int      `json:"lease_ttl_seconds"`
	Capabilities          []string `json:"capabilities"`
}

// Heartbeat 表示 Worker 发送的最小可用性信号。
type Heartbeat struct {
	// WorkerID 是发送该信号的 Worker 传输标识。
	WorkerID string `json:"worker_id"`

	// AgentRunID 是当前执行中的可选关联。
	AgentRunID string `json:"agent_run_id,omitempty"`

	// LeaseTokenSHA256 是 Lease 的不回显摘要。
	LeaseTokenSHA256 string `json:"lease_token_sha256,omitempty"`
}

// HeartbeatResponse 是 Daemon 返回的续租观察结果。
type HeartbeatResponse struct {
	WorkerID        string `json:"worker_id"`
	LeaseExpiresAt  string `json:"lease_expires_at,omitempty"`
	LeaseRenewed    bool   `json:"lease_renewed"`
	WorkerAvailable bool   `json:"worker_available"`
}

// PullRequest 表示 Worker 请求一个已授权 Assignment。
type PullRequest struct {
	WorkerID string `json:"worker_id"`
}

// PullResponse 使用 null 表示没有 Assignment，禁止用空 object 兼容。
type PullResponse struct {
	Assignment *Assignment `json:"assignment"`
}

// Assignment 表示 Daemon 下发给 Worker 的最小执行关联信息。
type Assignment struct {
	// AgentRunID 关联被分配的 AgentRun 传输标识。
	AgentRunID string `json:"agent_run_id"`

	// LeaseToken 是由边界持有者解释的不透明租约标识。
	LeaseToken string `json:"lease_token"`

	// WorkspacePath 指定 Worker 执行时使用的 Workspace 路径。
	WorkspacePath string `json:"workspace_path"`

	// Runtime 指定 Worker 应使用的 Runtime 标识。
	Runtime string `json:"runtime"`

	// ResultMode 只描述结果采集方式，不授予 Worker 生命周期权威。
	ResultMode string `json:"result_mode,omitempty"`

	// LeaseExpiresAt 是本次 Lease 的 UTC RFC3339 时间。
	LeaseExpiresAt string `json:"lease_expires_at,omitempty"`

	// Instruction 是有界执行输入，不包含控制面凭据。
	Instruction string `json:"instruction,omitempty"`

	// BeforeRevision 是 Daemon 认可的执行前 revision。
	BeforeRevision string `json:"before_revision,omitempty"`

	// Attempt 是不可复用 AgentRun 的尝试编号。
	Attempt int `json:"attempt,omitempty"`

	// WorkspaceID 是对外可观察的稳定标识；WorkspacePath 只在内部传输。
	WorkspaceID string `json:"workspace_id,omitempty"`

	// InputArtifacts 是输入证据摘要，不携带物理路径。
	InputArtifacts []ArtifactSummary `json:"input_artifacts,omitempty"`
}

// ArtifactSummary 描述 Assignment 输入 Artifact 的稳定摘要。
type ArtifactSummary struct {
	Kind      string `json:"kind"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
	MediaType string `json:"media_type,omitempty"`
}

// Report 表示 Worker 提交的一次执行结果关联信息。
type Report struct {
	// AgentRunID 关联产生该结果的 AgentRun 传输标识。
	AgentRunID string `json:"agent_run_id"`

	// LeaseToken 携带该结果对应的不透明租约标识。
	LeaseToken string `json:"lease_token"`

	// Outcome 携带 Worker 对执行结果的边界表达。
	Outcome Outcome `json:"outcome"`

	// Attempt 固定 Assignment 的当前尝试；零值保持旧 DTO 兼容。
	Attempt int `json:"attempt,omitempty"`

	// ExitCode 是 Worker 观察到的进程退出码。
	ExitCode *int `json:"exit_code,omitempty"`

	// StartedAt 和 CompletedAt 使用 UTC RFC3339Nano。
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`

	// AfterRevision 是 Worker 观察到的执行后 HEAD。
	AfterRevision string `json:"after_revision,omitempty"`

	// FailureReason 是 Worker 观察到的受限失败摘要，不承载堆栈或命令行。
	FailureReason string `json:"failure_reason,omitempty"`

	// GuardFindings 是独立 Guard 观察到的越界事实。
	GuardFindings []string `json:"guard_findings,omitempty"`

	// Artifacts 是执行证据与可选 Planning candidate 的有界 payload。
	Artifacts []Artifact `json:"artifacts,omitempty"`

	// CaptureFailures 记录独立采集失败，不使用 Runtime 自报替代。
	CaptureFailures []CaptureFailure `json:"capture_failures,omitempty"`
}

// Artifact 是 Report 中的有界证据内容。
type Artifact struct {
	Kind          string `json:"kind"`
	ContentBase64 string `json:"content_base64"`
	SHA256        string `json:"sha256"`
	SizeBytes     int64  `json:"size_bytes"`
	Truncated     bool   `json:"truncated"`
	CaptureError  string `json:"capture_error,omitempty"`
}

// CaptureFailure 描述某个证据阶段无法独立采集的原因。
type CaptureFailure struct {
	Kind  string `json:"kind"`
	Stage string `json:"stage"`
	Error string `json:"error"`
}

// ReportResponse 是 Daemon 对 Report 的有限处理结果。
type ReportResponse struct {
	Disposition     string `json:"disposition"`
	ErrorCode       string `json:"error_code,omitempty"`
	AgentRunID      string `json:"agent_run_id,omitempty"`
	LeaseState      string `json:"lease_state,omitempty"`
	RetrySameReport bool   `json:"retry_same_report"`
}

// ErrorResponse 是 Worker Protocol 的稳定错误边界。
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DecodeStrict 解码一个有界、单值、无未知字段、无重复 key 的 JSON object。
func DecodeStrict(body []byte, destination any) error {
	if len(body) == 0 || len(body) > MaxBodyBytes {
		return errors.New("worker protocol body is empty or too large")
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("worker protocol body must be a JSON object")
	}
	if err := validateUniqueJSON(trimmed); err != nil {
		return fmt.Errorf("validate worker protocol JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode worker protocol JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("worker protocol body contains multiple JSON values")
		}
		return fmt.Errorf("decode worker protocol trailing value: %w", err)
	}
	return nil
}

func validateUniqueJSON(value []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(value))
	if err := walkJSON(decoder); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}

func walkJSON(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch delimiter := token.(type) {
	case json.Delim:
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, keyErr := decoder.Token()
				if keyErr != nil {
					return keyErr
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("object key is not a string")
				}
				if key != strings.ToLower(key) {
					return fmt.Errorf("JSON key %q is not canonical lowercase", key)
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate JSON key %q", key)
				}
				seen[key] = struct{}{}
				if err := walkJSON(decoder); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := walkJSON(decoder); err != nil {
					return err
				}
			}
		default:
			return errors.New("unexpected JSON delimiter")
		}
		_, err = decoder.Token()
		return err
	default:
		return nil
	}
}

// Validate 检查 Register 的传输级必填字段。
func (r Register) Validate() error {
	if strings.TrimSpace(r.WorkerID) == "" || strings.TrimSpace(r.ProtocolVersion) == "" {
		return errors.New("worker register requires worker_id and protocol_version")
	}
	return nil
}

// Validate 检查 Heartbeat 的传输级必填字段。
func (h Heartbeat) Validate() error {
	if strings.TrimSpace(h.WorkerID) == "" {
		return errors.New("worker heartbeat requires worker_id")
	}
	return nil
}

// Validate 检查 PullRequest 的传输级必填字段。
func (p PullRequest) Validate() error {
	if strings.TrimSpace(p.WorkerID) == "" {
		return errors.New("worker pull requires worker_id")
	}
	return nil
}

// Validate 检查 Report 的传输级必填字段。
func (r Report) Validate() error {
	if strings.TrimSpace(r.AgentRunID) == "" || strings.TrimSpace(r.LeaseToken) == "" || strings.TrimSpace(string(r.Outcome)) == "" {
		return errors.New("worker report requires agent_run_id, lease_token and outcome")
	}
	return nil
}
