import axios from 'axios'
import { safeNumber } from '../utils/format'

// Relative URL — proxied by nginx (production) or vite (dev).
// DO NOT set VITE_API_BASE_URL to '/api' — just to '' or the backend origin.
const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || ''

// The /api prefix is used consistently. Nginx strips this prefix before proxying.
const API_PREFIX = '/api'
const IS_DEV = import.meta.env.DEV

const client = axios.create({
  baseURL: API_BASE_URL + API_PREFIX,
  timeout: 8000,
  headers: { Accept: 'application/json' },
})

// ---------- Error classes ----------
export class ApiError extends Error {
  constructor(code, message, status = 0, retryable = false) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
    this.retryable = retryable
  }
}

// ---------- Helpers ----------
function isHtmlResponse(data) {
  if (typeof data !== 'string') return false
  const trimmed = data.trim().toLowerCase()
  return trimmed.startsWith('<!doctype') || trimmed.startsWith('<html')
}

function normalizeLocation(raw, fallbackVehicleId = '') {
  if (!raw || typeof raw !== 'object') return null
  const candidate = raw.data ?? raw.location ?? raw

  const lat = safeNumber(candidate.latitude ?? candidate.lat)
  const lon = safeNumber(candidate.longitude ?? candidate.lon ?? candidate.lng)
  const ts = safeNumber(candidate.timestamp ?? candidate.ts ?? candidate.time)

  if (lat === null || lon === null || ts === null) {
    if (IS_DEV) {
      console.warn('[normalizeLocation] missing fields:', { raw, lat, lon, ts })
    }
    return null
  }

  return {
    vehicle_id: String(
      candidate.vehicle_id ?? candidate.vehicleId ?? candidate.id ?? fallbackVehicleId,
    ),
    latitude: lat,
    longitude: lon,
    timestamp: ts,
  }
}

function normalizeHistory(raw, fallbackVehicleId) {
  if (!raw || typeof raw !== 'object') {
    return { vehicle_id: fallbackVehicleId, count: 0, has_more: false, next_cursor: 0, data: [] }
  }
  const rows = Array.isArray(raw.data) ? raw.data : []
  const normalized = rows
    .map((r) => normalizeLocation(r, fallbackVehicleId))
    .filter(Boolean)

  return {
    vehicle_id: String(raw.vehicle_id ?? fallbackVehicleId),
    count: safeNumber(raw.count, normalized.length) ?? normalized.length,
    has_more: Boolean(raw.has_more),
    next_cursor: safeNumber(raw.next_cursor, 0) ?? 0,
    data: normalized,
  }
}

function classifyError(err, fallbackMsg) {
  if (err?.response?.data && isHtmlResponse(err.response.data)) {
    return new ApiError(
      'NGINX_FALLBACK',
      'Request to /api/ was caught by the nginx SPA fallback. Check nginx.conf.',
      err.response.status ?? 0,
    )
  }

  const status = err?.response?.status

  if (status === 404) {
    return new ApiError('NOT_FOUND', 'Vehicle has no location data yet.', 404)
  }
  if (status === 400) {
    return new ApiError(
      'BAD_REQUEST',
      err?.response?.data?.error ?? 'Invalid parameters.',
      400,
    )
  }
  if (status === 429) {
    return new ApiError('RATE_LIMIT', 'Too many requests. Please try again later.', 429, true)
  }
  if (status >= 500) {
    return new ApiError(
      'SERVER',
      err?.response?.data?.error ?? 'Backend error.',
      status,
      true,
    )
  }
  if (err?.code === 'ECONNABORTED' || err?.code === 'ETIMEDOUT') {
    return new ApiError('TIMEOUT', 'Backend timed out.', 0, true)
  }
  if (err?.name === 'AbortError' || err?.code === 'ERR_CANCELED') {
    return new ApiError('CANCELLED', 'Request was cancelled.', 0, false)
  }
  if (!err?.response) {
    return new ApiError('NETWORK', 'Failed to connect to the backend.', 0, true)
  }

  return new ApiError(
    'SERVER',
    err?.response?.data?.error ?? fallbackMsg ?? 'Backend error.',
    status ?? 0,
    false,
  )
}

// ---------- Public API ----------

/**
 * GET /vehicles/{id}/location
 * @param {string} vehicleId
 * @param {AbortSignal} [signal]
 */
export async function getLatestLocation(vehicleId, signal) {
  try {
    const { data } = await client.get(
      `/vehicles/${encodeURIComponent(vehicleId)}/location`,
      { signal },
    )

    if (isHtmlResponse(data)) {
      throw new ApiError('NGINX_FALLBACK', 'API returned an HTML response.', 200)
    }

    const loc = normalizeLocation(data, vehicleId)
    if (!loc) {
      throw new ApiError(
        'INVALID_RESPONSE',
        `Backend response is missing required fields. Got: ${JSON.stringify(data).slice(0, 200)}`,
        200,
      )
    }
    return loc
  } catch (err) {
    if (err instanceof ApiError) throw err
    throw classifyError(err, 'Failed to fetch the latest location.')
  }
}

/**
 * GET /vehicles/{id}/history?start=&end=&limit=
 */
export async function getHistory(vehicleId, start, end, limit = 1000, signal) {
  try {
    const { data } = await client.get(
      `/vehicles/${encodeURIComponent(vehicleId)}/history`,
      { params: { start, end, limit }, signal },
    )

    if (isHtmlResponse(data)) {
      throw new ApiError('NGINX_FALLBACK', 'API returned an HTML response.', 200)
    }

    return normalizeHistory(data, vehicleId)
  } catch (err) {
    if (err instanceof ApiError) throw err
    throw classifyError(err, 'Failed to load history.')
  }
}

/**
 * GET /readyz — readiness probe (checks PostgreSQL + RabbitMQ)
 */
export async function getReady(signal) {
  try {
    const { data } = await client.get('/readyz', { signal })
    return { ok: true, data }
  } catch (err) {
    return {
      ok: false,
      error: err?.response?.data ?? err?.message ?? 'unknown',
      status: err?.response?.status ?? 0,
    }
  }
}

/**
 * GET /geofences — list of geofences served by the backend.
 * Single source of truth is configs/config.yaml (section geofence.items).
 * Frontend MUST NOT hardcode geofence coordinates.
 */
export async function getGeofences(signal) {
  try {
    const { data } = await client.get('/geofences', { signal })

    if (isHtmlResponse(data)) {
      throw new ApiError('NGINX_FALLBACK', 'API returned an HTML response.', 200)
    }
    if (!Array.isArray(data)) {
      throw new ApiError('INVALID_RESPONSE', 'Geofences response is not an array.', 200)
    }

    // Normalize: backend uses radius_meters, some clients may send radius.
    return data
      .map((g) => {
        const lat = safeNumber(g?.latitude)
        const lon = safeNumber(g?.longitude)
        const radius = safeNumber(g?.radius_meters ?? g?.radius)
        if (lat === null || lon === null || radius === null) return null
        return {
          id: String(g?.id ?? ''),
          name: String(g?.name ?? g?.id ?? ''),
          latitude: lat,
          longitude: lon,
          radius_meters: radius,
        }
      })
      .filter(Boolean)
  } catch (err) {
    if (err instanceof ApiError) throw err
    throw classifyError(err, 'Failed to load geofences.')
  }
}