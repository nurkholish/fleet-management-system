import { memo } from 'react'
import StatusBadge from './StatusBadge'
import { SkeletonCard } from './Skeleton'
import {
  safeFixed,
  safeTime,
  formatRelativeTime,
  secondsAgo,
} from '../utils/format'

function LatestLocationCardView({ location, loading, error }) {
  if (loading && !location) {
    return <SkeletonCard />
  }

  if (error && !location) {
    return (
      <div className="card">
        <div className="card-header">
          <h2>Latest Location</h2>
          <StatusBadge variant="err">Error</StatusBadge>
        </div>
        <p className="error">{error}</p>
      </div>
    )
  }

  if (!location) {
    return (
      <div className="card">
        <div className="card-header">
          <h2>Latest Location</h2>
          <StatusBadge variant="warn">No data</StatusBadge>
        </div>
        <p className="muted">No data available for this vehicle yet.</p>
      </div>
    )
  }

  const age = secondsAgo(location.timestamp) ?? 0
  const variant = age < 10 ? 'ok' : age < 60 ? 'warn' : 'err'

  return (
    <div className="card">
      <div className="card-header">
        <h2>Latest Location</h2>
        <StatusBadge variant={variant}>
          {formatRelativeTime(location.timestamp)}
        </StatusBadge>
      </div>

      <dl className="kv">
        <div>
          <dt>Vehicle ID</dt>
          <dd>
            <code>{location.vehicle_id || '—'}</code>
          </dd>
        </div>
        <div>
          <dt>Latitude</dt>
          <dd>{safeFixed(location.latitude)}</dd>
        </div>
        <div>
          <dt>Longitude</dt>
          <dd>{safeFixed(location.longitude)}</dd>
        </div>
        <div>
          <dt>Timestamp</dt>
          <dd>{safeTime(location.timestamp)}</dd>
        </div>
      </dl>
    </div>
  )
}

export default memo(LatestLocationCardView)