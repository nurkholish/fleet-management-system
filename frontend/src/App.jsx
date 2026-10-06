import { useCallback, useEffect, useMemo, useState } from 'react'
import Header from './components/Header'
import LatestLocationCard from './components/LatestLocationCard'
import HistoryTable from './components/HistoryTable'
import Map from './components/Map'
import ErrorBoundary from './components/ErrorBoundary'
import HealthIndicator from './components/HealthIndicator'
import { useVehicleLocation } from './hooks/useVehicleLocation'
import { useHistory } from './hooks/useHistory'

const DEFAULT_VEHICLE = 'B1234XYZ'
const HISTORY_WINDOW_HOURS = 6

const pad = (n) => String(n).padStart(2, '0')

function toLocalInput(date) {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(
    date.getDate(),
  )}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function defaultRange() {
  const now = new Date()
  const from = new Date(now.getTime() - HISTORY_WINDOW_HOURS * 60 * 60 * 1000)
  return { start: toLocalInput(from), end: toLocalInput(now) }
}

export default function App() {
  const [vehicleIdInput, setVehicleIdInput] = useState(DEFAULT_VEHICLE)
  const [vehicleId, setVehicleId] = useState(DEFAULT_VEHICLE) // debounced
  const [range, setRange] = useState(defaultRange)

  // Debounce the vehicleId input — 500ms.
  useEffect(() => {
    const id = setTimeout(() => {
      setVehicleId(vehicleIdInput.trim().toUpperCase())
    }, 500)
    return () => clearTimeout(id)
  }, [vehicleIdInput])

  // Reset the history UI when the vehicle changes.
  useEffect(() => {
    setRange(defaultRange())
  }, [vehicleId])

  const { location: latest, loading: latestLoading, error: latestError, status } =
    useVehicleLocation(vehicleId)

  const historyRange = useMemo(() => {
    const start = Math.floor(new Date(range.start).getTime() / 1000)
    const end = Math.floor(new Date(range.end).getTime() / 1000)
    return { start, end }
  }, [range])

  const { history, loading: historyLoading, error: historyError, refresh: refreshHistory } =
    useHistory(vehicleId, historyRange, 1000)

  const handleVehicleChange = useCallback((next) => {
    setVehicleIdInput(next)
  }, [])

  return (
    <div className="app">
      <Header
        vehicleId={vehicleIdInput}
        onVehicleChange={handleVehicleChange}
        status={status}
      />

      <main className="grid">
        <section className="col map-col">
          <ErrorBoundary>
            <Map latest={latest} history={history?.data} />
          </ErrorBoundary>
        </section>

        <section className="col">
          <ErrorBoundary>
            <LatestLocationCard
              location={latest}
              loading={latestLoading}
              error={latestError}
            />
          </ErrorBoundary>

          <ErrorBoundary>
            <HistoryTable
              data={history}
              loading={historyLoading}
              error={historyError}
              range={range}
              onRangeChange={setRange}
              onRefresh={refreshHistory}
            />
          </ErrorBoundary>
        </section>
      </main>

      <footer className="app-footer">
        <span>Technical Test — Backend Engineer fleet-management-system</span>
        <HealthIndicator />
      </footer>
    </div>
  )
}