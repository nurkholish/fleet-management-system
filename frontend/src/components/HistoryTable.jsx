import { memo, useMemo } from 'react'
import { safeFixed, safeTime } from '../utils/format'

function HistoryTable({ data, loading, error, range, onRangeChange, onRefresh }) {
  const rows = useMemo(() => {
    const arr = Array.isArray(data?.data) ? data.data : []
    return arr.slice().reverse()
  }, [data])

  return (
    <div className="card">
      <div className="card-header">
        <h2>Location History</h2>
        <div className="actions">
          <input
            type="datetime-local"
            value={range.start}
            onChange={(e) =>
              onRangeChange({ ...range, start: e.target.value })
            }
            aria-label="Start time"
          />
          <span className="muted" aria-hidden="true">
            →
          </span>
          <input
            type="datetime-local"
            value={range.end}
            onChange={(e) => onRangeChange({ ...range, end: e.target.value })}
            aria-label="End time"
          />
          <button onClick={onRefresh} disabled={loading}>
            {loading ? '…' : 'Refresh'}
          </button>
        </div>
      </div>

      {error && <p className="error">{error}</p>}

      {!error && (
        <>
          <p className="muted">
            Total: {data?.count ?? 0} points{data?.has_more ? ' (partial)' : ''}
          </p>
          <div className="table-wrapper">
            <table>
              <thead>
                <tr>
                  <th style={{ width: 60 }}>#</th>
                  <th>Latitude</th>
                  <th>Longitude</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {rows.length === 0 && !loading && (
                  <tr>
                    <td colSpan={4} className="table-empty">
                      No data in this time range.
                    </td>
                  </tr>
                )}
                {rows.map((r, i) => (
                  <tr key={`${r.timestamp}-${r.latitude}-${r.longitude}`}>
                    <td>{rows.length - i}</td>
                    <td>{safeFixed(r.latitude)}</td>
                    <td>{safeFixed(r.longitude)}</td>
                    <td>{safeTime(r.timestamp)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  )
}

export default memo(HistoryTable)