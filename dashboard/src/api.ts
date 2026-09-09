export type Availability = 'available' | 'not_yet_available'

export interface PageInfo {
  has_more: boolean
  next_cursor?: string
}

export interface AgentRun {
  agent_run_id: string
  change_id: string
  stage: string
  attempt: number
  run_kind?: string
  source_revision?: string
  status: string
  outcome: string
  artifacts: Array<{ artifact_ref_id: string; role: string; ordinal: number }>
  started_at: string
  completed_at?: string | null
}

export interface ProjectSummary {
	project_id: string
	repository_root: string
	created_at: string
	change_count?: number
	active_change_count?: number
}

export interface ProjectListResponse extends PageInfo {
  projects: ProjectSummary[]
}

export interface ProjectSummaryResponse {
  project: ProjectSummary
}

export interface ChangeSummary {
  change_id: string
  project_id: string
  stage: string
  status: string
  version: number
  base_revision: string
  latest_agent_run?: AgentRun | null
  created_at: string
  updated_at: string
}

/** ChangeCreateRequest 是创建 Change 时发送给 Daemon 的最小写入边界。 */
export interface ChangeCreateRequest {
  repository_path: string
  intent: string
}

/** ChangeCreateResponse 是 Daemon 接受创建请求后的权威 Change 回执。 */
export interface ChangeCreateResponse {
  change: ChangeSummary & { repository_root: string }
}

export interface ProjectChangesResponse extends PageInfo {
  project_id: string
  changes: ChangeSummary[]
}

export interface ArtifactRef {
  artifact_ref_id: string
  artifact_id: string
  role: string
  ordinal: number
  kind?: string
  schema_version?: string
  summary?: string
  source_revision?: string
  input_artifact_ref_ids?: string[]
  raw_log_artifact_ref_ids?: string[]
}

export interface ChangeEvent {
  event_id: string
  change_id: string
  sequence: number
  type: string
  occurred_at: string
  actor: string
  artifact_ref_ids: string[]
  agent_run_id?: string | null
  decision_id?: string | null
  ticket_graph_id?: string | null
}

export interface HumanDecision {
  decision_id: string
  change_id: string
  decision: string
  actor: string
  reason: string
  created_at: string
}

export interface TicketGraph {
  graph_id: string
  change_id: string
  project_id: string
  base_revision: string
  plan_artifact: ArtifactRef
  draft_artifact: ArtifactRef
  ticketize_agent_run_id: string
  generator_name: string
  generator_version: string
  created_at: string
  tickets: Array<{
    ticket_id: string
    ordinal: number
    title: string
    scope: string
    acceptance_criteria: string[]
  }>
  dependencies: Array<{ dependent_ticket_id: string; blocker_ticket_id: string }>
  structural_frontier: string[]
}

export interface ExecutionTicket {
  ticket_id: string
  ordinal: number
  title: string
  state: string
  gate_status?: string
  verification_status?: string
  commit_status?: string
  verification_evidence_id?: string
  verification_command_statuses?: string[]
  verification_criterion_outcomes?: string[]
  input_revision?: string
  commit_before_revision?: string
  commit_after_revision?: string
}

export interface ExecutionReadModel {
  execution_session_id: string
  change_id: string
  status: string
  workspace_branch: string
  base_revision: string
  input_revision: string
  tickets: ExecutionTicket[]
  stage?: string
  change_status?: string
  version?: number
  pending_intent_ids?: string[]
  candidate_revision?: string
  final_verification?: string
  final_evidence_id?: string
}

export interface ObservationSection {
  availability: Availability
  reason_code?: string
  reason_summary?: string
}

export interface ChangeObservation {
  schema_version: string
  observed_at: string
  project: { project_id: string; created_at: string }
  change: {
    change_id: string
    project_id: string
    stage: string
    status: string
    version: number
    base_revision: string
    intent_artifact: ArtifactRef
    latest_agent_run?: AgentRun | null
    created_at: string
    updated_at: string
  }
  lifecycle: ObservationSection & { stage: string; status: string; version: number }
  ticket_graph: ObservationSection & { graph?: TicketGraph }
  execution: ObservationSection & { execution?: ExecutionReadModel }
  trace: ObservationSection & { events: ChangeEvent[]; runs: AgentRun[]; decisions: HumanDecision[] }
  artifacts: ObservationSection & {
    artifacts: Array<ArtifactRef & { byte_length: number; media_type: string }>
  }
  health: ObservationSection & {
    daemon_ready: boolean
    workers: Array<{
      worker_id: string
      protocol_version: string
      status: string
      capabilities: string[]
      last_heartbeat_at?: string
    }>
  }
  available_actions: string[]
}

export interface NeedsHumanItem {
  project_id: string
  change_id: string
  ticket_id?: string
  execution_scope?: string
  reason_code: string
  reason_summary: string
  available_actions: string[]
  change_version: number
  required_at: string
  evidence_artifact_ref_ids: string[]
}

export interface NeedsHumanResponse extends PageInfo {
  items: NeedsHumanItem[]
}

export interface ErrorEnvelope {
  code: string
  message: string
}

export class APIError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, payload: ErrorEnvelope) {
    super(payload.message)
    this.name = 'APIError'
    this.status = status
    this.code = payload.code
  }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...options,
    headers: {
      Accept: 'application/json',
      ...options.headers,
    },
  })
  if (!response.ok) {
    let payload: ErrorEnvelope = { code: 'http_error', message: `请求失败（${response.status}）。` }
    try {
      payload = (await response.json()) as ErrorEnvelope
    } catch {
      // 非 JSON 错误沿用稳定的 HTTP fallback 文案。
    }
    throw new APIError(response.status, payload)
  }
  return (await response.json()) as T
}

export function getJSON<T>(path: string, signal: AbortSignal): Promise<T> {
  return request<T>(path, { signal })
}

export function sendCommand(
  changeID: string,
  command: string,
  expectedVersion: number,
  idempotencyKey: string,
): Promise<unknown> {
  return request(`/v1/changes/${encodeURIComponent(changeID)}/commands`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify({ command, expected_version: expectedVersion }),
  })
}

export function sendDecision(
  changeID: string,
  decision: string,
  expectedVersion: number,
  reason: string,
  idempotencyKey: string,
): Promise<unknown> {
  return request(`/v1/changes/${encodeURIComponent(changeID)}/decisions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify({ decision, expected_version: expectedVersion, reason: reason || undefined }),
  })
}

/** DaemonStatusResponse 是 Dashboard 顶部可展示的本机 Daemon 权威状态。 */
export interface DaemonStatusResponse {
  daemon_readiness: boolean
}

/** createChange 仅提交已注册 Project 的 repository root 与未经改写的 Intent。 */
export function createChange(requestBody: ChangeCreateRequest, idempotencyKey: string): Promise<ChangeCreateResponse> {
  return request<ChangeCreateResponse>('/v1/changes', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify(requestBody),
  })
}
