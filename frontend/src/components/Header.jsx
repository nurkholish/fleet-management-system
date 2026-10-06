import StatusBadge from './StatusBadge'

export default function Header({ vehicleId, onVehicleChange, status }) {
  const statusMap = {
    connected: { variant: 'ok', label: 'Connected' },
    reconnecting: { variant: 'warn', label: 'Reconnecting' },
    offline: { variant: 'err', label: 'Offline' },
    idle: { variant: 'muted', label: 'Idle' },
  }
  const cfg = statusMap[status] ?? statusMap.idle

  return (
    <header className="app-header">
      <div className="brand">
        <span className="brand-logo" aria-hidden="true">
          🚌
        </span>
        <div className="brand-text">
          <h1>fleet-management-system Fleet</h1>
          <p className="subtitle">Real-time vehicle tracking</p>
        </div>
      </div>

      <div className="header-controls">
        <label className="vehicle-selector">
          <span>Vehicle</span>
          <input
            type="text"
            value={vehicleId}
            onChange={(e) => onVehicleChange(e.target.value.trim())}
            placeholder="B1234XYZ"
            spellCheck={false}
            autoComplete="off"
          />
        </label>
        <StatusBadge variant={cfg.variant}>{cfg.label}</StatusBadge>
      </div>
    </header>
  )
}