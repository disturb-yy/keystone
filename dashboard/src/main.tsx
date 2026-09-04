import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import './styles.css'

function App() {
  return (
    <main className="shell" aria-labelledby="page-title">
      <section className="card">
        <p className="eyebrow">Keystone</p>
        <h1 id="page-title">工程骨架</h1>
        <p>这是当前前端工程的静态构建入口。</p>
        <p>本页面用于确认工程能够安装、静态检查并生成生产构建产物。</p>
      </section>
    </main>
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
