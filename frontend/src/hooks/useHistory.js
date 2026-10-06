import { useCallback, useEffect, useRef, useState } from 'react'
import { getHistory } from '../api/client'
import { usePolling } from './usePolling'

export function useHistory(vehicleId, range, limit = 1000) {
  const [history, setHistory] = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)

  const rangeRef = useRef(range)
  rangeRef.current = range

  // Skip the immediate fetch on first mount — usePolling already ticks
  // immediately on mount. Prevents a double request on mount.
  const isFirstRender = useRef(true)

  // Reset state when the vehicle changes.
  useEffect(() => {
    setHistory(null)
    setError(null)
    setLoading(false)
  }, [vehicleId])

  const fetch = useCallback(
    async (signal) => {
      if (!vehicleId) return
      const { start, end } = rangeRef.current
      if (!start || !end || end <= start) return

      setLoading(true)
      try {
        const data = await getHistory(vehicleId, start, end, limit, signal)
        setHistory(data)
        setError(null)
      } catch (err) {
        if (err?.code === 'CANCELLED') return
        setError(err?.message ?? 'Failed to load history.')
      } finally {
        setLoading(false)
      }
    },
    [vehicleId, limit],
  )

  usePolling(fetch, 10000, Boolean(vehicleId), 60000)

  // Immediate refresh when the range changes (skip first mount).
  useEffect(() => {
    if (isFirstRender.current) {
      isFirstRender.current = false
      return
    }
    if (!vehicleId || !range.start || !range.end) return
    if (range.end <= range.start) return

    const controller = new AbortController()
    fetch(controller.signal)
    return () => controller.abort()
  }, [vehicleId, range.start, range.end, fetch])

  const refresh = useCallback(
    () => fetch(new AbortController().signal),
    [fetch],
  )

  return { history, loading, error, refresh }
}