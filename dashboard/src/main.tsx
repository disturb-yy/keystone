import { StrictMode, useCallback, useState } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Outlet, Route, Routes } from 'react-router-dom'

import './styles.css'

import { DashboardShell, type ShellContext } from './components'
import { useRefreshStream, type StreamStatus } from './hooks'
import { ChangeDetailPage, CreateChangePage, NeedsHumanPage, ProjectDetailPage, ProjectsPage } from './pages'

function ShellRoute({ refreshVersion, streamStatus }: ShellContext): ReactElement {
  return (
    <DashboardShell refreshVersion={refreshVersion}>
      <div className="shell-stream-context"><Outlet context={{ refreshVersion, streamStatus } satisfies ShellContext} /></div>
    </DashboardShell>
  )
}

function App(): ReactElement {
  const [refreshVersion, setRefreshVersion] = useState(0)
  const [streamStatus, setStreamStatus] = useState<StreamStatus>('connecting')
  const onRefresh = useCallback(() => setRefreshVersion((value) => value + 1), [])
  const onStatus = useCallback((status: StreamStatus) => setStreamStatus(status), [])
  useRefreshStream(onRefresh, onStatus)
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<ShellRoute refreshVersion={refreshVersion} streamStatus={streamStatus} />}>
          <Route path="/" element={<ProjectsPage />} />
          <Route path="/projects/:project_id" element={<ProjectDetailPage />} />
          <Route path="/changes/new" element={<CreateChangePage />} />
          <Route path="/changes/:change_id" element={<ChangeDetailPage />} />
          <Route path="/needs-human" element={<NeedsHumanPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}

const rootElement = document.getElementById('root')

if (!rootElement) {
  throw new Error('找不到应用挂载节点。')
}

createRoot(rootElement).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
