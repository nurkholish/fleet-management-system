import { useCallback, useEffect, useRef, useState } from 'react'
import { getLatestLocation } from '../api/client'
import { usePolling } from './usePolling'


export function useVehicleLocation(vehicleId) {
  const [location, setLocation] = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  const [status, setStatus] = useState('idle')

  const hadSuccessRef = useRef(false)

  // Skip the immediate fetch on first mount — usePolling handles it.
  const isFirstRender = useRef(true)

  const fetch = useCallback(
    async (signal) => {
      if (!vehicleId) return

      if (!hadSuccessRef.current) setLoading(true)

      try {
        const loc = await getLatestLocation(vehicleId, signal)
        setLocation(loc)
        setError(null)
        setStatus('connected')
        hadSuccessRef.current = true
      } catch (err) {
        if (err?.code === 'CANCELLED') return

        if (err?.code === 'NOT_FOUND') {
          setError('Vehicle has no location data yet.')
          setStatus('reconnecting')
        } else if (err?.code === 'NETWORK' || err?.code === 'TIMEOUT') {
          setError('Backend is unreachable.')
          setStatus('offline')
        } else if (err?.code === 'NGINX_FALLBACK') {
          setError('Nginx misconfigured — /api/ is not proxied to the backend.')
          setStatus('offline')
        } else {
          setError(err?.message ?? 'Failed to fetch data.')
          setStatus('offline')
        }
      } finally {
        setLoading(false)
      }
    },
    [vehicleId],
  )

  // Background polling.
  usePolling(fetch, 2000, Boolean(vehicleId), 30000)

  // Immediate fetch when the vehicle changes (skip the first mount).
  useEffect(() => {
    // Reset state for the new vehicle.
    hadSuccessRef.current = false
    setLocation(null)
    setError(null)
    setLoading(false)
    setStatus('idle')

    if (isFirstRender.current) {
      isFirstRender.current = false
      return // usePolling will trigger an immediate tick on mount
    }
    if (!vehicleId) return

    const controller = new AbortController()
    fetch(controller.signal)
    return () => controller.abort()
  }, [vehicleId, fetch])

  return { location, loading, error, status }
}