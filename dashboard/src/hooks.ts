import { useEffect, useState } from 'react'

import { APIError, getJSON } from './api'

export interface QueryState<T> {
  data?: T
  loading: boolean
  error?: APIError | Error
  stale: boolean
}

export function useQuery<T>(path: string, refreshKey: number): QueryState<T> {
  const [state, setState] = useState<QueryState<T>>({ loading: true, stale: false })

  useEffect(() => {
    const controller = new AbortController()
    let active = true
    setState((current) => ({ ...current, loading: true, error: undefined, stale: Boolean(current.data) }))

    getJSON<T>(path, controller.signal)
      .then((data) => {
        if (active) {
          setState({ data, loading: false, stale: false })
        }
      })
      .catch((error: unknown) => {
        if (active && !(error instanceof DOMException && error.name === 'AbortError')) {
          setState((current) => ({ ...current, loading: false, error: error as APIError | Error, stale: Boolean(current.data) }))
        }
      })

    return () => {
      active = false
      controller.abort()
    }
  }, [path, refreshKey])

  return state
}

export type StreamStatus = 'connecting' | 'connected' | 'reconnecting' | 'disconnected'

export function useRefreshStream(onRefresh: () => void, onStatus: (status: StreamStatus) => void): void {
  useEffect(() => {
    const source = new EventSource('/v1/updates')
    onStatus('connecting')
    source.onopen = () => {
      onStatus('connected')
      onRefresh()
    }
    source.addEventListener('refresh', onRefresh)
    source.onerror = () => onStatus(source.readyState === EventSource.CONNECTING ? 'reconnecting' : 'disconnected')
    return () => {
      source.close()
      onStatus('disconnected')
    }
  }, [onRefresh, onStatus])
}
