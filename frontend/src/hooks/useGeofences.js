import { useEffect, useState } from 'react'
import { getGeofences } from '../api/client'

const REFRESH_MS = 60000

export function useGeofences() {
  const [geofences, setGeofences] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  useEffect(() => {
    let cancelled = false
    const controller = new AbortController()

    async function load() {
      try {
        const list = await getGeofences(controller.signal)
        if (!cancelled) {
          setGeofences(list)
          setError(null)
        }
      } catch (err) {
        if (cancelled) return
        if (err?.code === 'CANCELLED') return
        setError(err?.message ?? 'Failed to load geofences.')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    load()
    const id = setInterval(() => {
      if (document.visibilityState === 'visible') load()
    }, REFRESH_MS)

    return () => {
      cancelled = true
      clearInterval(id)
      controller.abort()
    }
  }, [])

  return { geofences, loading, error }
}