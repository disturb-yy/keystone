// Package controlplane 定义 Client 与 Control Plane Daemon 之间的传输契约。
package controlplane

import "errors"

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
	if key == "" {
		return ErrEmptyIdempotencyKey
	}

	return nil
}

// String 返回幂等键的原始 Header 值。
func (key IdempotencyKey) String() string {
	return string(key)
}
