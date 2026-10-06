import { useEffect, useState } from 'react'
import { getReady } from '../api/client'

export default function HealthIndicator() {
  const [state, setState] = useState({ ok: null, error: null })

  useEffect(() => {
    let cancelled = false
    const controller = new AbortController()

    async function check() {
      const res = await getReady(controller.signal)
      if (!cancelled) setState(res)
    }

    check()
    const id = setInterval(() => {
      if (document.visibilityState === 'visible') check()
    }, 15000)

    return () => {
      cancelled = true
      clearInterval(id)
      controller.abort()
    }
  }, [])

  if (state.ok === null) {
    return <span className="health-indicator">checking…</span>
  }

  if (!state.ok) {
    return (
      <span
        className="health-indicator health-indicator--err"
        title={JSON.stringify(state.error)}
      >
        ● degraded
      </span>
    )
  }

  return (
    <span
      className="health-indicator health-indicator--ok"
      title="PostgreSQL + RabbitMQ ready"
    >
      ● ready
    </span>
  )
}