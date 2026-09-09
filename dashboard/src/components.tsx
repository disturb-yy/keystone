import type { ReactElement, ReactNode } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { Alert, Button, Card, Empty, Layout, Menu, Tag, Typography } from 'tdesign-react'
import { DashboardIcon, ErrorCircleIcon, FactCheckIcon, RefreshIcon } from 'tdesign-icons-react'

import { APIError, type Availability, type ObservationSection } from './api'
import type { StreamStatus } from './hooks'

export interface ShellContext {
  refreshVersion: number
  streamStatus: StreamStatus
}

export function DashboardShell({ children }: { children: ReactNode }): ReactElement {
  const location = useLocation()
  const navigate = useNavigate()
  const menuValue = location.pathname.startsWith('/needs-human') ? '/needs-human' : location.pathname === '/' ? '/' : '/'
  return (
    <Layout className="app-layout">
      <Layout.Header className="app-header">
        <div className="brand-lockup">
          <div className="brand-mark" aria-hidden="true"><DashboardIcon /></div>
          <div>
            <Typography.Title level="h5" className="brand-title">Keystone</Typography.Title>
            <Typography.Text className="brand-subtitle">Control Plane Observation</Typography.Text>
          </div>
        </div>
        <div className="header-status">
          <span className="status-dot" aria-hidden="true" />
          <span>本机 Daemon</span>
        </div>
      </Layout.Header>
      <Layout>
        <Layout.Aside className="app-aside">
          <Menu value={menuValue} onChange={(value) => navigate(String(value))}>
            <Menu.MenuItem value="/" icon={<DashboardIcon />}>Projects</Menu.MenuItem>
            <Menu.MenuItem value="/needs-human" icon={<FactCheckIcon />}>Needs Human</Menu.MenuItem>
          </Menu>
        </Layout.Aside>
        <Layout.Content className="app-content">
          <div className="content-frame">{children}</div>
        </Layout.Content>
      </Layout>
    </Layout>
  )
}

export function PageHeader({ eyebrow, title, description, actions }: { eyebrow?: string; title: string; description?: string; actions?: ReactNode }): ReactElement {
  return (
    <div className="page-header">
      <div>
        {eyebrow && <Typography.Text className="eyebrow">{eyebrow}</Typography.Text>}
        <Typography.Title level="h2" className="page-title">{title}</Typography.Title>
        {description && <Typography.Paragraph className="page-description">{description}</Typography.Paragraph>}
      </div>
      {actions && <div className="page-actions">{actions}</div>}
    </div>
  )
}

export function StreamBanner({ status }: { status: StreamStatus }): ReactElement | null {
  if (status === 'connected') return null
  const message = status === 'connecting' ? '正在连接刷新流。页面仍以 Query 响应为准。' : status === 'reconnecting' ? '刷新流正在重连。最近一次成功数据已标记为 stale。' : '刷新流已断开。请手动刷新或等待下一次连接。'
  return <Alert className="stream-banner" theme={status === 'disconnected' ? 'warning' : 'info'} icon={<ErrorCircleIcon />} message={message} />
}

export function QueryNotice({ loading, stale, error, onRetry }: { loading: boolean; stale: boolean; error?: APIError | Error; onRetry?: () => void }): ReactElement | null {
  if (loading && !stale) return <div className="query-loading" role="status"><span className="loading-bar" />正在读取 Daemon Query…</div>
  if (!error && !stale) return null
  if (error) {
    const conflict = error instanceof APIError && error.status === 409
    return (
      <Alert theme={conflict ? 'warning' : 'error'} icon={<ErrorCircleIcon />} title={conflict ? '观察快照已陈旧' : 'Query 暂时不可用'} message={conflict ? '服务器拒绝了陈旧操作，请重新读取当前快照。' : error.message} operation={onRetry ? <Button variant="text" icon={<RefreshIcon />} onClick={onRetry}>重新读取</Button> : undefined} />
    )
  }
  return <Alert theme="warning" icon={<ErrorCircleIcon />} message="当前展示的是最近一次成功 Query，已标记为 stale。" />
}

export function StatusTag({ value }: { value: string }): ReactElement {
  const theme = value === 'active' || value === 'succeeded' || value === 'available' || value === 'registered' ? 'success' : value === 'human_required' || value === 'paused' || value === 'running' || value === 'not_yet_available' ? 'warning' : value === 'cancelled' || value === 'failed' || value === 'revoked' ? 'danger' : 'default'
  return <Tag theme={theme} variant="light-outline">{value}</Tag>
}

export function SectionCard({ title, section, children, action }: { title: string; section: ObservationSection; children: ReactNode; action?: ReactNode }): ReactElement {
  const unavailable = section.availability === ('not_yet_available' satisfies Availability)
  return (
    <Card className="section-card" title={title} actions={action}>
      {unavailable ? <Alert theme="info" title="尚未形成" message={`${section.reason_summary ?? '上游事实尚未形成。'}（${section.reason_code ?? 'not_yet_available'}）`} /> : children}
    </Card>
  )
}

export function EmptyState({ description }: { description: string }): ReactElement {
  return <Empty image={<FactCheckIcon />} description={description} />
}

export function ErrorState({ error, onRetry }: { error?: APIError | Error; onRetry: () => void }): ReactElement {
  return <div className="error-state"><ErrorCircleIcon /><Typography.Title level="h4">无法读取当前页面</Typography.Title><Typography.Paragraph>{error?.message ?? 'Daemon Query 暂时不可用。'}</Typography.Paragraph><Button theme="primary" icon={<RefreshIcon />} onClick={onRetry}>重新读取</Button></div>
}

export function LinkButton({ to, children }: { to: string; children: ReactNode }): ReactElement {
  return <Link className="link-button" to={to}>{children}</Link>
}
