/**
 * Convert a value to a number. Returns `fallback` if invalid.
 * @param {*} value
 * @param {number|null} fallback
 * @returns {number|null}
 */
export function safeNumber(value, fallback = null) {
  if (value === null || value === undefined) return fallback
  const n = Number(value)
  return Number.isFinite(n) ? n : fallback
}

/**
 * Format a number with fixed digits. Returns `fallback` if invalid.
 * @param {*} value
 * @param {number} digits
 * @param {string} fallback
 * @returns {string}
 */
export function safeFixed(value, digits = 6, fallback = '—') {
  const n = safeNumber(value)
  return n === null ? fallback : n.toFixed(digits)
}

/**
 * Format a Unix timestamp (seconds) into a local Asia/Jakarta string.
 * @param {*} ts   Unix timestamp (seconds)
 * @param {Intl.DateTimeFormatOptions} options
 * @returns {string}
 */
export function safeTime(ts, options = {}) {
  const n = safeNumber(ts)
  if (n === null || n <= 0) return '—'
  try {
    return new Date(n * 1000).toLocaleString('en-GB', {
      timeZone: 'Asia/Jakarta',
      hour12: false,
      ...options,
    })
  } catch {
    return '—'
  }
}

/**
 * Seconds elapsed since the given timestamp. Returns null if invalid.
 * @param {*} ts   Unix timestamp (seconds)
 * @returns {number|null}
 */
export function secondsAgo(ts) {
  const n = safeNumber(ts)
  if (n === null) return null
  return Math.max(0, Math.floor(Date.now() / 1000) - n)
}

/**
 * Relative time in English.
 * @param {*} ts   Unix timestamp (seconds)
 * @returns {string}
 */
export function formatRelativeTime(ts) {
  const sec = secondsAgo(ts)
  if (sec === null) return '—'
  if (sec < 5) return 'just now'
  if (sec < 60) return `${sec}s ago`
  if (sec < 3600) return `${Math.floor(sec / 60)}m ago`
  if (sec < 86400) return `${Math.floor(sec / 3600)}h ago`
  return `${Math.floor(sec / 86400)}d ago`
}