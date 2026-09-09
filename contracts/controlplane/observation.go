package controlplane

// PageInfo 是 Dashboard 列表 Query 的服务端分页元数据。
type PageInfo struct {
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// ProjectSummaryDTO 是 Dashboard 可展示的 Project 身份和计数摘要。
type ProjectSummaryDTO struct {
	ProjectID         string `json:"project_id"`
	RepositoryRoot    string `json:"repository_root"`
	CreatedAt         string `json:"created_at"`
	ChangeCount       int    `json:"change_count"`
	ActiveChangeCount int    `json:"active_change_count"`
}

// ProjectListResponse 是 Projects inventory Query 的成功响应。
type ProjectListResponse struct {
	Projects []ProjectSummaryDTO `json:"projects"`
	PageInfo
}

// ProjectSummaryResponse 是 Project Detail Query 的成功响应。
type ProjectSummaryResponse struct {
	Project ProjectSummaryDTO `json:"project"`
}

// ChangeSummaryDTO 是 Project-scoped Change list 的有界摘要。
type ChangeSummaryDTO struct {
	ChangeID       string       `json:"change_id"`
	ProjectID      string       `json:"project_id"`
	Stage          string       `json:"stage"`
	Status         string       `json:"status"`
	Version        int          `json:"version"`
	BaseRevision   string       `json:"base_revision"`
	LatestAgentRun *AgentRunDTO `json:"latest_agent_run"`
	CreatedAt      string       `json:"created_at"`
	UpdatedAt      string       `json:"updated_at"`
}

// ProjectChangesResponse 是 Project-scoped Change list 的成功响应。
type ProjectChangesResponse struct {
	ProjectID string             `json:"project_id"`
	Changes   []ChangeSummaryDTO `json:"changes"`
	PageInfo
}

// ObservationSection 是组合 Query section 的显式可用性标记。
type ObservationSection struct {
	Availability  string `json:"availability"`
	ReasonCode    string `json:"reason_code,omitempty"`
	ReasonSummary string `json:"reason_summary,omitempty"`
}

// LifecycleObservation 是 Change 阶段、状态和版本的组合摘要。
type LifecycleObservation struct {
	ObservationSection
	Stage   string `json:"stage"`
	Status  string `json:"status"`
	Version int    `json:"version"`
}

// ObservationProjectDTO 是 Change Observation 中不含路径的 Project 身份摘要。
type ObservationProjectDTO struct {
	ProjectID string `json:"project_id"`
	CreatedAt string `json:"created_at"`
}

// ObservationChangeDTO 是 Change Observation 中不含 Repository root 的 Change 摘要。
type ObservationChangeDTO struct {
	ChangeID       string         `json:"change_id"`
	ProjectID      string         `json:"project_id"`
	Stage          string         `json:"stage"`
	Status         string         `json:"status"`
	Version        int            `json:"version"`
	BaseRevision   string         `json:"base_revision"`
	IntentArtifact ArtifactRefDTO `json:"intent_artifact"`
	LatestAgentRun *AgentRunDTO   `json:"latest_agent_run"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

// TicketGraphObservation 是 Canonical Ticket Graph 的有界组合 section。
type TicketGraphObservation struct {
	ObservationSection
	Graph *TicketGraphReadModel `json:"graph,omitempty"`
}

// ExecutionObservation 是 bounded Execution ReadModel 的组合 section。
type ExecutionObservation struct {
	ObservationSection
	Execution *ExecutionReadModel `json:"execution,omitempty"`
}

// TraceObservation 保持 Event、AgentRun 和 HumanDecision 的独立关联。
type TraceObservation struct {
	ObservationSection
	Events    []ChangeEventDTO   `json:"events"`
	Runs      []AgentRunDTO      `json:"runs"`
	Decisions []HumanDecisionDTO `json:"decisions"`
}

// ArtifactObservationDTO 是 ArtifactRef 的安全身份和摘要投影。
type ArtifactObservationDTO struct {
	ArtifactRefID        string   `json:"artifact_ref_id"`
	ArtifactID           string   `json:"artifact_id"`
	Role                 string   `json:"role"`
	Ordinal              int      `json:"ordinal"`
	Kind                 string   `json:"kind,omitempty"`
	SchemaVersion        string   `json:"schema_version,omitempty"`
	Summary              string   `json:"summary,omitempty"`
	SourceRevision       string   `json:"source_revision,omitempty"`
	ByteLength           int64    `json:"byte_length"`
	MediaType            string   `json:"media_type"`
	InputArtifactRefIDs  []string `json:"input_artifact_ref_ids,omitempty"`
	RawLogArtifactRefIDs []string `json:"raw_log_artifact_ref_ids,omitempty"`
}

// ArtifactsObservation 是有界 ArtifactRef 摘要 section。
type ArtifactsObservation struct {
	ObservationSection
	Artifacts []ArtifactObservationDTO `json:"artifacts"`
}

// WorkerHealthDTO 是 Daemon 已授权的 Worker 注册和心跳摘要。
type WorkerHealthDTO struct {
	WorkerID        string   `json:"worker_id"`
	ProtocolVersion string   `json:"protocol_version"`
	Status          string   `json:"status"`
	Capabilities    []string `json:"capabilities"`
	LastHeartbeatAt string   `json:"last_heartbeat_at,omitempty"`
}

// HealthObservation 是 Daemon/Worker health 的安全投影。
type HealthObservation struct {
	ObservationSection
	DaemonReady bool              `json:"daemon_ready"`
	Workers     []WorkerHealthDTO `json:"workers"`
}

// ChangeObservationReadModel 是 Change Detail 的统一、有界观察 envelope。
type ChangeObservationReadModel struct {
	SchemaVersion    string                 `json:"schema_version"`
	ObservedAt       string                 `json:"observed_at"`
	Project          ObservationProjectDTO  `json:"project"`
	Change           ObservationChangeDTO   `json:"change"`
	Lifecycle        LifecycleObservation   `json:"lifecycle"`
	TicketGraph      TicketGraphObservation `json:"ticket_graph"`
	Execution        ExecutionObservation   `json:"execution"`
	Trace            TraceObservation       `json:"trace"`
	Artifacts        ArtifactsObservation   `json:"artifacts"`
	Health           HealthObservation      `json:"health"`
	AvailableActions []string               `json:"available_actions"`
}

// NeedsHumanItemDTO 是 Daemon 已标记的待人工项目。
type NeedsHumanItemDTO struct {
	ProjectID              string   `json:"project_id"`
	ChangeID               string   `json:"change_id"`
	TicketID               string   `json:"ticket_id,omitempty"`
	ExecutionScope         string   `json:"execution_scope,omitempty"`
	ReasonCode             string   `json:"reason_code"`
	ReasonSummary          string   `json:"reason_summary"`
	AvailableActions       []string `json:"available_actions"`
	ChangeVersion          int      `json:"change_version"`
	RequiredAt             string   `json:"required_at"`
	EvidenceArtifactRefIDs []string `json:"evidence_artifact_ref_ids"`
}

// NeedsHumanResponse 是 Needs Human inventory Query 的成功响应。
type NeedsHumanResponse struct {
	Items []NeedsHumanItemDTO `json:"items"`
	PageInfo
}

// RefreshHint 是 SSE 仅用于唤醒重新 Query 的提示，不承载业务状态。
type RefreshHint struct {
	ResourceType string `json:"resource_type"`
	ProjectID    string `json:"project_id,omitempty"`
	ChangeID     string `json:"change_id,omitempty"`
}
