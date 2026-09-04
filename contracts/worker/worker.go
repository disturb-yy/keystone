// Package worker 定义 Keystone Daemon 与独立 Worker 之间的窄传输边界。
package worker

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

// Heartbeat 表示 Worker 发送的最小可用性信号。
type Heartbeat struct {
	// WorkerID 是发送该信号的 Worker 传输标识。
	WorkerID string `json:"worker_id"`
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
}

// Report 表示 Worker 提交的一次执行结果关联信息。
type Report struct {
	// AgentRunID 关联产生该结果的 AgentRun 传输标识。
	AgentRunID string `json:"agent_run_id"`

	// LeaseToken 携带该结果对应的不透明租约标识。
	LeaseToken string `json:"lease_token"`

	// Outcome 携带 Worker 对执行结果的边界表达。
	Outcome Outcome `json:"outcome"`
}
