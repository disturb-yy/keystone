import { useState } from 'react'
import type { ReactElement } from 'react'
import { Link, useOutletContext, useParams } from 'react-router-dom'
import type { PrimaryTableCol } from 'tdesign-react'
import { Alert, Button, Card, Drawer, Space, Statistic, Steps, Table, Tag, Textarea, Timeline, Typography } from 'tdesign-react'
import { RefreshIcon } from 'tdesign-icons-react'

import { APIError, sendCommand, sendDecision, type AgentRun, type ArtifactRef, type ChangeEvent, type ChangeObservation, type ChangeSummary, type HumanDecision, type NeedsHumanItem, type ProjectSummary, type TicketGraph } from './api'
import { useQuery } from './hooks'
import type { ShellContext } from './components'
import { EmptyState, ErrorState, LinkButton, PageHeader, QueryNotice, SectionCard, StatusTag, StreamBanner } from './components'

const lifecycleStages = ['Intent', 'Understand', 'Design', 'Plan', 'Ticketize', 'Execute', 'Verify', 'FinalVerify']

function usePageContext(): ShellContext {
  return useOutletContext<ShellContext>()
}

function formatTime(value?: string): string {
  if (!value) return '—'
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

function requestKey(): string {
  return typeof crypto.randomUUID === 'function' ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function ListPager({ hasMore, onNext }: { hasMore?: boolean; onNext: () => void }): ReactElement | null {
  if (!hasMore) return null
  return <div className="list-pager"><Button variant="outline" onClick={onNext}>读取下一页</Button></div>
}

export function ProjectsPage(): ReactElement {
  const { refreshVersion, streamStatus } = usePageContext()
  const [reload, setReload] = useState(0)
  const [cursor, setCursor] = useState('')
  const path = `/v1/projects?limit=50${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`
  const query = useQuery<{ projects: ProjectSummary[]; has_more: boolean; next_cursor?: string }>(path, refreshVersion + reload)
  const columns: PrimaryTableCol<ProjectSummary>[] = [
    { colKey: 'project_id', title: 'Project', cell: ({ row }) => <Link className="table-link" to={`/projects/${row.project_id}`}>{row.project_id}</Link> },
    { colKey: 'repository_root', title: 'Repository root', ellipsis: true },
    { colKey: 'change_count', title: 'Changes', align: 'right' },
    { colKey: 'active_change_count', title: 'Active', align: 'right', cell: ({ row }) => <Tag theme={(row.active_change_count ?? 0) > 0 ? 'primary' : 'default'}>{row.active_change_count ?? 0}</Tag> },
    { colKey: 'created_at', title: 'Registered', cell: ({ row }) => formatTime(row.created_at) },
  ]
  return (
    <>
      <StreamBanner status={streamStatus} />
      <PageHeader eyebrow="Inventory" title="Projects" description="浏览由本机 Daemon 注册的 Project，并进入其 Change 观察链。" actions={<Button variant="outline" icon={<RefreshIcon />} onClick={() => setReload((value) => value + 1)}>刷新</Button>} />
      <QueryNotice loading={query.loading} stale={query.stale} error={query.error} onRetry={() => setReload((value) => value + 1)} />
      {query.error && !query.data ? <ErrorState error={query.error} onRetry={() => setReload((value) => value + 1)} /> : query.data && query.data.projects.length === 0 ? <Card><EmptyState description="Daemon 尚未注册 Project。" /></Card> : query.data ? <Card className="table-card" title="Project inventory"><Table<ProjectSummary> rowKey="project_id" columns={columns} data={query.data.projects} disableDataPage bordered hover /></Card> : null}
      {query.data && <ListPager hasMore={query.data.has_more} onNext={() => setCursor(query.data?.next_cursor ?? '')} />}
    </>
  )
}

export function ProjectDetailPage(): ReactElement {
  const { project_id: projectID = '' } = useParams()
  const { refreshVersion, streamStatus } = usePageContext()
  const [reload, setReload] = useState(0)
  const [cursor, setCursor] = useState('')
  const projectQuery = useQuery<{ project: ProjectSummary }>(`/v1/projects/${encodeURIComponent(projectID)}`, refreshVersion + reload)
  const changesPath = `/v1/projects/${encodeURIComponent(projectID)}/changes?limit=50${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`
  const changesQuery = useQuery<{ changes: ChangeSummary[]; has_more: boolean; next_cursor?: string }>(changesPath, refreshVersion + reload)
  const columns: PrimaryTableCol<ChangeSummary>[] = [
    { colKey: 'change_id', title: 'Change', cell: ({ row }) => <Link className="table-link" to={`/changes/${row.change_id}`}>{row.change_id}</Link> },
    { colKey: 'stage', title: 'Stage' },
    { colKey: 'status', title: 'Status', cell: ({ row }) => <StatusTag value={row.status} /> },
    { colKey: 'version', title: 'Version', align: 'right' },
    { colKey: 'updated_at', title: 'Updated', cell: ({ row }) => formatTime(row.updated_at) },
  ]
  const project = projectQuery.data?.project
  return (
    <>
      <StreamBanner status={streamStatus} />
      <PageHeader eyebrow="Project detail" title={projectID} description="Project identity 与其 Change 摘要由 Daemon Query 提供。" actions={<Space><LinkButton to="/">返回 Projects</LinkButton><Button variant="outline" icon={<RefreshIcon />} onClick={() => setReload((value) => value + 1)}>刷新</Button></Space>} />
      <QueryNotice loading={projectQuery.loading || changesQuery.loading} stale={projectQuery.stale || changesQuery.stale} error={projectQuery.error ?? changesQuery.error} onRetry={() => setReload((value) => value + 1)} />
      {projectQuery.error && !project ? <ErrorState error={projectQuery.error} onRetry={() => setReload((value) => value + 1)} /> : project ? <div className="summary-grid"><Card title="Repository identity"><Typography.Text className="summary-label">Repository root</Typography.Text><Typography.Paragraph><code>{project.repository_root}</code></Typography.Paragraph><Space><Statistic title="Changes" value={project.change_count ?? changesQuery.data?.changes.length ?? 0} /><Statistic title="Active" value={project.active_change_count ?? 0} /></Space></Card><Card title="Registered"><Typography.Title level="h4">{formatTime(project.created_at)}</Typography.Title><Typography.Text theme="secondary">Project identity 由 Manifest/Daemon 权威记录维护。</Typography.Text></Card></div> : null}
      {changesQuery.error && !changesQuery.data ? <ErrorState error={changesQuery.error} onRetry={() => setReload((value) => value + 1)} /> : changesQuery.data && changesQuery.data.changes.length === 0 ? <Card title="Changes"><EmptyState description="这个 Project 尚未产生 Change。" /></Card> : changesQuery.data ? <Card className="table-card" title="Changes"><Table<ChangeSummary> rowKey="change_id" columns={columns} data={changesQuery.data.changes} disableDataPage bordered hover /></Card> : null}
      {changesQuery.data && <ListPager hasMore={changesQuery.data.has_more} onNext={() => setCursor(changesQuery.data?.next_cursor ?? '')} />}
    </>
  )
}

function LifecyclePanel({ observation }: { observation: ChangeObservation }): ReactElement {
  const current = Math.max(0, lifecycleStages.indexOf(observation.lifecycle.stage))
  return <Card className="section-card" title="Lifecycle" actions={<StatusTag value={observation.lifecycle.status} />}><Steps current={current}>{lifecycleStages.map((stage) => <Steps.StepItem key={stage} title={stage} />)}</Steps><div className="lifecycle-meta"><span>Stage: <strong>{observation.lifecycle.stage}</strong></span><span>Version: <strong>{observation.lifecycle.version}</strong></span><span>Observed: {formatTime(observation.observed_at)}</span></div></Card>
}

function ActionPanel({ observation, onComplete }: { observation: ChangeObservation; onComplete: () => void }): ReactElement | null {
  const [pending, setPending] = useState('')
  const [error, setError] = useState<Error>()
  if (observation.available_actions.length === 0) return null
  const perform = async (action: string): Promise<void> => {
    setPending(action)
    setError(undefined)
    try {
      if (action === 'retry' || (action === 'cancel' && observation.change.status === 'human_required')) {
        await sendDecision(observation.change.change_id, action, observation.change.version, '', requestKey())
      } else {
        await sendCommand(observation.change.change_id, action, observation.change.version, requestKey())
      }
      onComplete()
    } catch (caught) {
      setError(caught instanceof Error ? caught : new Error('操作失败。'))
    } finally {
      setPending('')
    }
  }
  return <Card className="action-card" title="Available actions"><Space>{observation.available_actions.map((action) => <Button key={action} theme={action === 'cancel' ? 'danger' : 'primary'} variant={action === 'cancel' ? 'outline' : 'base'} loading={pending === action} disabled={Boolean(pending)} onClick={() => void perform(action)}>{action}</Button>)}</Space>{error && <Alert className="inline-alert" theme={error instanceof APIError && error.status === 409 ? 'warning' : 'error'} message={error instanceof APIError && error.status === 409 ? '快照已陈旧，请重新读取后再操作。' : error.message} />}</Card>
}

function TicketGraphView({ graph }: { graph?: TicketGraph }): ReactElement {
  if (!graph) return <EmptyState description="没有 Canonical Ticket Graph。" />
  const columns: PrimaryTableCol<TicketGraph['tickets'][number]>[] = [
    { colKey: 'ordinal', title: '#', width: 64 },
    { colKey: 'ticket_id', title: 'Ticket ID', ellipsis: true },
    { colKey: 'title', title: 'Title' },
    { colKey: 'scope', title: 'Scope', ellipsis: true },
    { colKey: 'acceptance_criteria', title: 'Criteria', cell: ({ row }) => row.acceptance_criteria.length },
  ]
  return <><div className="metadata-grid"><Typography.Text>Graph: <code>{graph.graph_id}</code></Typography.Text><Typography.Text>Generator: {graph.generator_name} {graph.generator_version}</Typography.Text><Typography.Text>Structural frontier: {graph.structural_frontier.length || 'empty'}</Typography.Text></div><Table columns={columns} data={graph.tickets} rowKey="ticket_id" disableDataPage bordered hover /></>
}

function ExecutionView({ execution }: { execution?: ChangeObservation['execution']['execution'] }): ReactElement {
  if (!execution) return <EmptyState description="Execution 尚未开始。" />
  const columns: PrimaryTableCol<typeof execution.tickets[number]>[] = [
    { colKey: 'ordinal', title: '#', width: 64 },
    { colKey: 'ticket_id', title: 'Ticket ID', ellipsis: true },
    { colKey: 'title', title: 'Title' },
    { colKey: 'state', title: 'State', cell: ({ row }) => <StatusTag value={row.state} /> },
    { colKey: 'verification_status', title: 'Verify', cell: ({ row }) => row.verification_status ? <StatusTag value={row.verification_status} /> : '—' },
    { colKey: 'commit_status', title: 'Commit', cell: ({ row }) => row.commit_status ? <StatusTag value={row.commit_status} /> : '—' },
  ]
  return <><div className="metadata-grid"><Typography.Text>Session: <code>{execution.execution_session_id}</code></Typography.Text><Typography.Text>Status: <StatusTag value={execution.status} /></Typography.Text><Typography.Text>Candidate revision: {execution.candidate_revision || '—'}</Typography.Text></div><Table columns={columns} data={execution.tickets} rowKey="ticket_id" disableDataPage bordered hover /></>
}

function TraceView({ events, runs, decisions }: { events: ChangeEvent[]; runs: AgentRun[]; decisions: HumanDecision[] }): ReactElement {
  return <div className="trace-layout"><div><Typography.Title level="h5">Events</Typography.Title>{events.length === 0 ? <EmptyState description="暂无 Event。" /> : <Timeline>{events.map((event) => <Timeline.Item key={event.event_id} label={`#${event.sequence} · ${formatTime(event.occurred_at)}`}><strong>{event.type}</strong><Typography.Text theme="secondary"> · {event.actor || 'daemon'}</Typography.Text></Timeline.Item>)}</Timeline>}</div><div><Typography.Title level="h5">Agent Runs</Typography.Title>{runs.length === 0 ? <EmptyState description="暂无 AgentRun。" /> : runs.map((run) => <div className="trace-row" key={run.agent_run_id}><StatusTag value={run.status} /><span>{run.stage} · attempt {run.attempt}</span><Typography.Text theme="secondary">{run.outcome || 'running'}</Typography.Text></div>)}</div><div><Typography.Title level="h5">Human Decisions</Typography.Title>{decisions.length === 0 ? <EmptyState description="暂无 HumanDecision。" /> : decisions.map((decision) => <div className="trace-row" key={decision.decision_id}><StatusTag value={decision.decision} /><span>{decision.actor}</span><Typography.Text theme="secondary">{decision.reason || '—'}</Typography.Text></div>)}</div></div>
}

function ChangeObservationPage(): ReactElement {
  const { change_id: changeID = '' } = useParams()
  const { refreshVersion, streamStatus } = usePageContext()
  const [reload, setReload] = useState(0)
  const query = useQuery<ChangeObservation>(`/v1/changes/${encodeURIComponent(changeID)}/observation`, refreshVersion + reload)
  const observation = query.data
  return (
    <>
      <StreamBanner status={streamStatus} />
      <PageHeader eyebrow="Change detail" title={observation?.change.change_id ?? changeID} description="所有 section 都来自 Daemon Query；刷新提示不会携带或推导业务状态。" actions={<Space><LinkButton to={observation ? `/projects/${observation.project.project_id}` : '/'}>返回 Project</LinkButton><Button variant="outline" icon={<RefreshIcon />} onClick={() => setReload((value) => value + 1)}>刷新</Button></Space>} />
      <QueryNotice loading={query.loading} stale={query.stale} error={query.error} onRetry={() => setReload((value) => value + 1)} />
      {query.error && !observation ? <ErrorState error={query.error} onRetry={() => setReload((value) => value + 1)} /> : observation ? <div className="observation-stack"><ActionPanel observation={observation} onComplete={() => setReload((value) => value + 1)} /><LifecyclePanel observation={observation} /><SectionCard title="Canonical Ticket Graph" section={observation.ticket_graph}><TicketGraphView graph={observation.ticket_graph.graph} /></SectionCard><SectionCard title="Execution" section={observation.execution}><ExecutionView execution={observation.execution.execution} /></SectionCard><SectionCard title="Trace" section={observation.trace}><TraceView events={observation.trace.events} runs={observation.trace.runs} decisions={observation.trace.decisions} /></SectionCard><SectionCard title="Artifacts" section={observation.artifacts}>{observation.artifacts.artifacts.length === 0 ? <EmptyState description="暂无 ArtifactRef。" /> : <ArtifactTable artifacts={observation.artifacts.artifacts} />}</SectionCard><SectionCard title="Daemon / Worker Health" section={observation.health}><HealthTable observation={observation} /></SectionCard></div> : null}
    </>
  )
}

function ArtifactTable({ artifacts }: { artifacts: Array<ArtifactRef & { byte_length: number; media_type: string }> }): ReactElement {
  const columns: PrimaryTableCol<ArtifactRef & { byte_length: number; media_type: string }>[] = [
    { colKey: 'artifact_ref_id', title: 'Ref', ellipsis: true },
    { colKey: 'role', title: 'Role' },
    { colKey: 'kind', title: 'Kind', cell: ({ row }) => row.kind || '—' },
    { colKey: 'byte_length', title: 'Size', cell: ({ row }) => `${row.byte_length} B` },
    { colKey: 'summary', title: 'Summary', ellipsis: true, cell: ({ row }) => row.summary || '—' },
  ]
  return <Table columns={columns} data={artifacts} rowKey="artifact_ref_id" disableDataPage bordered hover />
}

function HealthTable({ observation }: { observation: ChangeObservation }): ReactElement {
  const columns: PrimaryTableCol<ChangeObservation['health']['workers'][number]>[] = [
    { colKey: 'worker_id', title: 'Worker', ellipsis: true },
    { colKey: 'status', title: 'Status', cell: ({ row }) => <StatusTag value={row.status} /> },
    { colKey: 'protocol_version', title: 'Protocol' },
    { colKey: 'capabilities', title: 'Capabilities', cell: ({ row }) => row.capabilities.join(', ') || '—' },
    { colKey: 'last_heartbeat_at', title: 'Last heartbeat', cell: ({ row }) => formatTime(row.last_heartbeat_at) },
  ]
  return <><Space align="center"><Tag theme={observation.health.daemon_ready ? 'success' : 'warning'}>{observation.health.daemon_ready ? 'Daemon ready' : 'Daemon unavailable'}</Tag><Typography.Text theme="secondary">Worker health 由 Daemon 直接提供，不在浏览器计算 freshness。</Typography.Text></Space>{observation.health.workers.length === 0 ? <EmptyState description="暂无 Worker 注册记录。" /> : <Table columns={columns} data={observation.health.workers} rowKey="worker_id" disableDataPage bordered hover />}</>
}

export function ChangeDetailPage(): ReactElement {
  return <ChangeObservationPage />
}

function NeedsHumanDrawer({ item, onClose, onComplete }: { item?: NeedsHumanItem; onClose: () => void; onComplete: () => void }): ReactElement {
  const [reason, setReason] = useState('')
  const [pending, setPending] = useState('')
  const [error, setError] = useState<Error>()
  const submit = async (decision: string): Promise<void> => {
    if (!item) return
    setPending(decision)
    setError(undefined)
    try {
      await sendDecision(item.change_id, decision, item.change_version, reason, requestKey())
      onComplete()
      onClose()
    } catch (caught) {
      setError(caught instanceof Error ? caught : new Error('Decision 提交失败。'))
    } finally {
      setPending('')
    }
  }
  return <Drawer header="Human decision" visible={Boolean(item)} onClose={onClose} size="420px"><div className="drawer-content">{item && <><Typography.Title level="h4">{item.change_id}</Typography.Title><Typography.Paragraph>{item.reason_summary}</Typography.Paragraph><div className="evidence-list"><Typography.Text strong>Evidence references</Typography.Text>{item.evidence_artifact_ref_ids.length === 0 ? <Typography.Text theme="secondary">没有关联 evidence。</Typography.Text> : item.evidence_artifact_ref_ids.map((ref) => <code key={ref}>{ref}</code>)}</div><Textarea value={reason} onChange={(value) => setReason(String(value))} placeholder="可选：说明本次人工决定原因" autosize={{ minRows: 4, maxRows: 8 }} /> <Space className="drawer-actions"><Button theme="primary" loading={pending === 'retry'} disabled={Boolean(pending)} onClick={() => void submit('retry')}>Retry</Button><Button theme="danger" variant="outline" loading={pending === 'cancel'} disabled={Boolean(pending)} onClick={() => void submit('cancel')}>Cancel</Button></Space>{error && <Alert theme={error instanceof APIError && error.status === 409 ? 'warning' : 'error'} message={error instanceof APIError && error.status === 409 ? '快照已陈旧，请重新读取。' : error.message} />}</>}</div></Drawer>
}

export function NeedsHumanPage(): ReactElement {
  const { refreshVersion, streamStatus } = usePageContext()
  const [reload, setReload] = useState(0)
  const [cursor, setCursor] = useState('')
  const [selected, setSelected] = useState<NeedsHumanItem>()
  const path = `/v1/needs-human?limit=50${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`
  const query = useQuery<{ items: NeedsHumanItem[]; has_more: boolean; next_cursor?: string }>(path, refreshVersion + reload)
  const columns: PrimaryTableCol<NeedsHumanItem>[] = [
    { colKey: 'change_id', title: 'Change', cell: ({ row }) => <Link className="table-link" to={`/changes/${row.change_id}`}>{row.change_id}</Link>, ellipsis: true },
    { colKey: 'reason_code', title: 'Reason', cell: ({ row }) => <><Tag theme="warning">{row.reason_code}</Tag><div>{row.reason_summary}</div></> },
    { colKey: 'change_version', title: 'Version', align: 'right' },
    { colKey: 'required_at', title: 'Required at', cell: ({ row }) => formatTime(row.required_at) },
    { colKey: 'actions', title: 'Actions', cell: ({ row }) => <Button size="small" onClick={() => setSelected(row)}>Review</Button> },
  ]
  return (
    <>
      <StreamBanner status={streamStatus} />
      <PageHeader eyebrow="Governance queue" title="Needs Human" description="只显示 Daemon 明确标记为 human_required 的 Change，不根据超时或客户端状态推导。" actions={<Button variant="outline" icon={<RefreshIcon />} onClick={() => setReload((value) => value + 1)}>刷新</Button>} />
      <QueryNotice loading={query.loading} stale={query.stale} error={query.error} onRetry={() => setReload((value) => value + 1)} />
      {query.error && !query.data ? <ErrorState error={query.error} onRetry={() => setReload((value) => value + 1)} /> : query.data && query.data.items.length === 0 ? <Card><EmptyState description="当前没有待人工处理的 Change。" /></Card> : query.data ? <Card className="table-card" title="Human-required inventory"><Table<NeedsHumanItem> rowKey="change_id" columns={columns} data={query.data.items} disableDataPage bordered hover /></Card> : null}
      {query.data && <ListPager hasMore={query.data.has_more} onNext={() => setCursor(query.data?.next_cursor ?? '')} />}
      <NeedsHumanDrawer item={selected} onClose={() => setSelected(undefined)} onComplete={() => setReload((value) => value + 1)} />
    </>
  )
}
