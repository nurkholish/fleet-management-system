import { memo, useEffect, useMemo } from 'react'
import {
  MapContainer,
  TileLayer,
  Marker,
  Popup,
  Polyline,
  Circle,
  useMap,
} from 'react-leaflet'
import L from 'leaflet'
import { safeNumber, safeFixed } from '../utils/format'
import { useGeofences } from '../hooks/useGeofences'

import markerIcon2x from 'leaflet/dist/images/marker-icon-2x.png'
import markerIcon from 'leaflet/dist/images/marker-icon.png'
import markerShadow from 'leaflet/dist/images/marker-shadow.png'

delete L.Icon.Default.prototype._getIconUrl
L.Icon.Default.mergeOptions({
  iconRetinaUrl: markerIcon2x,
  iconUrl: markerIcon,
  shadowUrl: markerShadow,
})

const DEFAULT_CENTER = [-6.2088, 106.8456]
const MAP_ZOOM = 14

function isValidCoord(p) {
  const lat = safeNumber(p?.latitude)
  const lon = safeNumber(p?.longitude)
  return (
    lat !== null && lon !== null && Math.abs(lat) <= 90 && Math.abs(lon) <= 180
  )
}

// Recenter ONLY if the vehicle moved more than 5m from the last center.
function RecenterMap({ center }) {
  const map = useMap()
  useEffect(() => {
    if (!center) return
    try {
      const current = map.getCenter()
      const dist = map.distance(current, center)
      if (dist > 5) {
        map.setView(center, map.getZoom(), { animate: true })
      }
    } catch (err) {
      if (import.meta.env.DEV) console.warn('[map recenter]', err)
    }
  }, [center, map])
  return null
}

function MapView({ latest, history }) {
  const hasValidLatest = isValidCoord(latest)
  const { geofences } = useGeofences()

  const center = useMemo(() => {
    if (hasValidLatest) {
      return [safeNumber(latest.latitude), safeNumber(latest.longitude)]
    }
    return DEFAULT_CENTER
  }, [hasValidLatest, latest])

  const positions = useMemo(() => {
    if (!Array.isArray(history)) return []
    return history
      .filter(isValidCoord)
      .map((p) => [safeNumber(p.latitude), safeNumber(p.longitude)])
  }, [history])

  return (
    <div className="map-wrapper">
      <MapContainer
        center={center}
        zoom={MAP_ZOOM}
        style={{ height: '100%', width: '100%' }}
        scrollWheelZoom
        preferCanvas
      >
        <TileLayer
          attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>'
          url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
          updateWhenIdle
        />

        {geofences.map((g) => (
          <Circle
            key={g.id}
            center={[g.latitude, g.longitude]}
            radius={g.radius_meters}
            pathOptions={{
              color: '#f97316',
              fillColor: '#fb923c',
              fillOpacity: 0.18,
            }}
          >
            <Popup>
              <strong>{g.name}</strong>
              <br />
              Geofence radius: {g.radius_meters}m
            </Popup>
          </Circle>
        ))}

        {positions.length > 1 && (
          <Polyline
            positions={positions}
            pathOptions={{ color: '#2563eb', weight: 3, opacity: 0.7 }}
          />
        )}

        {hasValidLatest && (
          <Marker
            position={[
              safeNumber(latest.latitude),
              safeNumber(latest.longitude),
            ]}
          >
            <Popup>
              <strong>{latest.vehicle_id || 'Vehicle'}</strong>
              <br />
              {safeFixed(latest.latitude, 5)}, {safeFixed(latest.longitude, 5)}
            </Popup>
          </Marker>
        )}

        <RecenterMap center={hasValidLatest ? center : null} />
      </MapContainer>
    </div>
  )
}

export default memo(MapView, (prev, next) => {
  return (
    prev.latest?.latitude === next.latest?.latitude &&
    prev.latest?.longitude === next.latest?.longitude &&
    prev.history?.length === next.history?.length
  )
})