import { useEffect, useRef } from 'react'

export function usePolling(callback, intervalMs, enabled = true, hiddenIntervalMs) {
  const savedCallback = useRef(callback)
  const runningRef = useRef(false)
  const abortRef = useRef(null)

  // Keep the callback fresh without re-running the effect.
  useEffect(() => {
    savedCallback.current = callback
  }, [callback])

  useEffect(() => {
    if (!enabled) return

    let cancelled = false
    let timerId = null

    const effectiveHidden = hiddenIntervalMs ?? Math.max(intervalMs, 30000)

    async function tick() {
      if (cancelled || runningRef.current) return
      runningRef.current = true

      // Abort the previous request if any.
      if (abortRef.current) abortRef.current.abort()
      const controller = new AbortController()
      abortRef.current = controller

      try {
        await savedCallback.current(controller.signal)
      } catch (err) {
        if (err?.name !== 'AbortError' && err?.code !== 'CANCELLED') {
          if (import.meta.env.DEV) {
            console.warn('[polling]', err?.message ?? err)
          }
        }
      } finally {
        runningRef.current = false
      }
    }

    function schedule() {
      const interval =
        document.visibilityState === 'hidden' ? effectiveHidden : intervalMs
      timerId = setTimeout(async () => {
        await tick()
        if (!cancelled) schedule()
      }, interval)
    }

    // Immediate first tick.
    tick().finally(() => {
      if (!cancelled) schedule()
    })

    // Resume with an immediate refresh when the tab becomes visible.
    function onVisibilityChange() {
      if (document.visibilityState === 'visible' && !cancelled) {
        if (timerId) clearTimeout(timerId)
        tick().finally(() => {
          if (!cancelled) schedule()
        })
      }
    }

    document.addEventListener('visibilitychange', onVisibilityChange)

    return () => {
      cancelled = true
      if (timerId) clearTimeout(timerId)
      document.removeEventListener('visibilitychange', onVisibilityChange)
      if (abortRef.current) abortRef.current.abort()
      runningRef.current = false
    }
    // ✅ Dependencies are primitives only — callback & hiddenIntervalMs via ref.
  }, [intervalMs, enabled, hiddenIntervalMs])
}