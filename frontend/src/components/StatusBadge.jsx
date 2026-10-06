// fleet-management-system/frontend/src/components/StatusBadge.jsx
export default function StatusBadge({ variant = 'muted', children }) {
  const map = {
    ok: 'badge badge-ok',
    warn: 'badge badge-warn',
    err: 'badge badge-err',
    muted: 'badge badge-muted',
  }
  return <span className={map[variant] ?? map.muted}>{children}</span>
}