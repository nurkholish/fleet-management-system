// fleet-management-system/frontend/src/components/Skeleton.jsx
export function SkeletonLine({ width = '100%', height = 14 }) {
  return <div className="skeleton" style={{ width, height }} />
}

export function SkeletonCard() {
  return (
    <div className="card">
      <SkeletonLine width="40%" height={12} />
      <div style={{ height: 12 }} />
      <SkeletonLine width="80%" height={20} />
      <div style={{ height: 6 }} />
      <SkeletonLine width="60%" height={20} />
    </div>
  )
}