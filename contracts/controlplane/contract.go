// Package controlplane 定义 Client 与 Control Plane Daemon 之间的传输契约。
package controlplane

import (
	"errors"
	"strings"
)

const (
	// VersionPrefix 是 Control Plane API 的稳定版本前缀。
	VersionPrefix = "/v1"

	// IdempotencyKeyHeader 是 mutating Command 使用的幂等键 HTTP Header 名称。
	IdempotencyKeyHeader = "Idempotency-Key"
)

// ErrorEnvelope 是 Control Plane API 的结构化错误响应。
//
// Code 和 Message 是必需的边界字段；RequestID 由服务端可选返回，供调用方
// 关联日志或诊断信息。该类型不承载业务数据或内部错误细节。
type ErrorEnvelope struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// HealthResponse 是 Control Plane 健康检查的最小传输响应。
//
// ready 的 HTTP 路由、状态码和 Daemon 生命周期含义由 HTTP Adapter 负责；
// Contract 只固定 JSON 字段。
type HealthResponse struct {
	Ready bool `json:"ready"`
}

// DaemonStatusResponse 是 GET /v1/daemon/status 的成功响应。
//
// 该类型只表达 Daemon 当前状态和持久化状态元数据的传输快照；数据库查询、
// readiness 判定和 HTTP 状态码由边界外的实现负责。
type DaemonStatusResponse struct {
	DatabasePath           string `json:"database_path"`
	SchemaMigrationVersion int    `json:"schema_migration_version"`
	DaemonReadiness        bool   `json:"daemon_readiness"`
	DaemonInstanceID       string `json:"daemon_instance_id"`
}

// DaemonStopRequest 是 POST /v1/daemon/stop 的请求。
//
// DaemonInstanceID 是请求中必需提供的实例标识；缺失或空值的校验由 HTTP
// Adapter 负责。该 Command 不在传输体中携带 Idempotency-Key。
type DaemonStopRequest struct {
	DaemonInstanceID string `json:"daemon_instance_id"`
}

// DaemonStopResponse 是 Daemon 接受停止请求后的成功响应。
//
// Accepted 表示当前实例已接受优雅停止请求；DaemonInstanceID 标识接受该
// 请求的当前实例。
type DaemonStopResponse struct {
	Accepted         bool   `json:"accepted"`
	DaemonInstanceID string `json:"daemon_instance_id"`
}

// ProjectInitRequest 是 Project 初始化 Command 的请求体。
type ProjectInitRequest struct {
	RepositoryPath string `json:"repository_path"`
}

// ProjectDTO 是 Control Plane 返回的 Project 快照。
type ProjectDTO struct {
	ProjectID      string `json:"project_id"`
	RepositoryRoot string `json:"repository_root"`
	ManifestPath   string `json:"manifest_path"`
	CreatedAt      string `json:"created_at"`
}

// ProjectInitResponse 是 Project 初始化成功响应。
type ProjectInitResponse struct {
	Project ProjectDTO `json:"project"`
}

// ProjectQueryResponse 是 Project Query 成功响应。
type ProjectQueryResponse struct {
	Project ProjectDTO `json:"project"`
}

// ProjectEventDTO 是 ProjectInitialized 的最小查询边界。
type ProjectEventDTO struct {
	EventID    string `json:"event_id"`
	ProjectID  string `json:"project_id"`
	Type       string `json:"type"`
	OccurredAt string `json:"occurred_at"`
}

// ProjectEventsResponse 是 Project Event Query 成功响应。
type ProjectEventsResponse struct {
	Events []ProjectEventDTO `json:"events"`
}

// ChangeCreateRequest 是 Change 创建的严格请求体。
type ChangeCreateRequest struct {
	RepositoryPath string `json:"repository_path"`
	Intent         string `json:"intent"`
}

// ChangeDTO 是 Change 当前状态的公开快照。
type ChangeDTO struct {
	ChangeID       string         `json:"change_id"`
	ProjectID      string         `json:"project_id"`
	RepositoryRoot string         `json:"repository_root"`
	Stage          string         `json:"stage"`
	Status         string         `json:"status"`
	Version        int            `json:"version"`
	BaseRevision   string         `json:"base_revision"`
	IntentArtifact ArtifactRefDTO `json:"intent_artifact"`
	LatestAgentRun *AgentRunDTO   `json:"latest_agent_run"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

// ChangeCreateResponse 是 Change 创建成功响应。
type ChangeCreateResponse struct {
	Change ChangeDTO `json:"change"`
}

// ChangeListResponse 是 Change 列表成功响应。
type ChangeListResponse struct {
	Changes []ChangeDTO `json:"changes"`
}

// ChangeCommandRequest 是 Pause、Resume、Cancel 请求体。
type ChangeCommandRequest struct {
	Command         string `json:"command"`
	ExpectedVersion int    `json:"expected_version"`
}

// ChangeCommandResponse 是普通 Change 命令成功响应。
type ChangeCommandResponse struct {
	Change ChangeDTO `json:"change"`
}

// HumanDecisionRequest 是 retry 或 cancel 人工决定请求体。
type HumanDecisionRequest struct {
	Decision        string `json:"decision"`
	ExpectedVersion int    `json:"expected_version"`
	Reason          string `json:"reason,omitempty"`
}

// HumanDecisionResponse 是人工决定成功响应。
type HumanDecisionResponse struct {
	Change ChangeDTO `json:"change"`
}

// ArtifactRefDTO 是 ArtifactRef 的稳定公开摘要。
type ArtifactRefDTO struct {
	ArtifactRefID string `json:"artifact_ref_id"`
	ArtifactID    string `json:"artifact_id"`
	Role          string `json:"role"`
	Ordinal       int    `json:"ordinal"`
}

// ArtifactDTO 是 Artifact 内容身份和媒体类型摘要。
type ArtifactDTO struct {
	ArtifactID string `json:"artifact_id"`
	SHA256     string `json:"sha256"`
	ByteLength int64  `json:"byte_length"`
	MediaType  string `json:"media_type"`
	CreatedAt  string `json:"created_at"`
}

// AgentRunArtifactDTO 是 AgentRun 输入、输出或失败证据的稳定关联摘要。
type AgentRunArtifactDTO struct {
	ArtifactRefID string `json:"artifact_ref_id"`
	Role          string `json:"role"`
	Ordinal       int    `json:"ordinal"`
}

// AgentRunDTO 是 AgentRun 的稳定公开摘要。
type AgentRunDTO struct {
	AgentRunID  string                `json:"agent_run_id"`
	ChangeID    string                `json:"change_id"`
	Stage       string                `json:"stage"`
	Attempt     int                   `json:"attempt"`
	Status      string                `json:"status"`
	Outcome     string                `json:"outcome"`
	Artifacts   []AgentRunArtifactDTO `json:"artifacts"`
	StartedAt   string                `json:"started_at"`
	CompletedAt *string               `json:"completed_at"`
}

// HumanDecisionDTO 是 HumanDecision 的稳定公开摘要。
type HumanDecisionDTO struct {
	DecisionID string `json:"decision_id"`
	ChangeID   string `json:"change_id"`
	Decision   string `json:"decision"`
	Actor      string `json:"actor"`
	Reason     string `json:"reason"`
	CreatedAt  string `json:"created_at"`
}

// ChangeEventDTO 是 Change Event 的固定查询边界。
type ChangeEventDTO struct {
	EventID        string   `json:"event_id"`
	ChangeID       string   `json:"change_id"`
	Sequence       int      `json:"sequence"`
	Type           string   `json:"type"`
	OccurredAt     string   `json:"occurred_at"`
	Actor          string   `json:"actor"`
	ArtifactRefIDs []string `json:"artifact_ref_ids"`
	AgentRunID     *string  `json:"agent_run_id"`
	DecisionID     *string  `json:"decision_id"`
}

// ChangeEventsResponse 是 Change Event Trace 成功响应。
type ChangeEventsResponse struct {
	Events []ChangeEventDTO `json:"events"`
}

// ChangeRunsResponse 是 AgentRun Trace 成功响应。
type ChangeRunsResponse struct {
	Runs []AgentRunDTO `json:"runs"`
}

// ChangeArtifactsResponse 是 ArtifactRef Trace 成功响应。
type ChangeArtifactsResponse struct {
	Artifacts []ArtifactRefDTO `json:"artifacts"`
}

// ChangeDecisionsResponse 是 HumanDecision Trace 成功响应。
type ChangeDecisionsResponse struct {
	Decisions []HumanDecisionDTO `json:"decisions"`
}

// ErrEmptyIdempotencyKey 表示幂等键为空。
var ErrEmptyIdempotencyKey = errors.New("idempotency key must not be empty")

// IdempotencyKey 是不透明的 HTTP 幂等键值。
//
// Contract 不解释或规范化键值内容；具体 mutating Command 是否要求该键由
// 后续用例决定。
type IdempotencyKey string

// ParseIdempotencyKey 将 HTTP Header 值解析为非空幂等键，并保留原始值。
func ParseIdempotencyKey(value string) (IdempotencyKey, error) {
	key := IdempotencyKey(value)
	if err := key.Validate(); err != nil {
		return "", err
	}

	return key, nil
}

// Validate 检查幂等键是否存在，不对不透明值施加格式约束。
func (key IdempotencyKey) Validate() error {
	if string(key) == "" {
		return ErrEmptyIdempotencyKey
	}

	return nil
}

// ValidateProjectID 检查小写 canonical UUIDv7 的 HTTP 路径表达。
func ValidateProjectID(value string) error {
	return ValidateUUIDv7(value, "project_id")
}

// ValidateChangeID 检查小写 canonical UUIDv7 的 Change 路径表达。
func ValidateChangeID(value string) error {
	return ValidateUUIDv7(value, "change_id")
}

// ValidateArtifactRefID 检查小写 canonical UUIDv7 的 ArtifactRef 路径表达。
func ValidateArtifactRefID(value string) error {
	return ValidateUUIDv7(value, "artifact_ref_id")
}

// ValidateUUIDv7 检查 Control Plane 路径中的小写 canonical UUIDv7。
func ValidateUUIDv7(value, field string) error {
	if len(value) != 36 || value != strings.ToLower(value) ||
		value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' ||
		value[14] != '7' {
		return errors.New(field + " must be a lowercase canonical UUIDv7")
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !strings.ContainsRune("0123456789abcdef", char) {
			return errors.New(field + " must be a lowercase canonical UUIDv7")
		}
	}
	if value[19] < '8' || value[19] > 'b' {
		return errors.New(field + " must use RFC 9562 variant bits")
	}
	return nil
}

// String 返回幂等键的原始 Header 值。
func (key IdempotencyKey) String() string {
	return string(key)
}
