<template>
  <div class="aircraft-map-wrap">
    <div ref="mapContainer" class="aircraft-map"></div>
    <Transition name="card">
      <div v-if="selectedAircraft" class="aircraft-info-card">
        <button class="card-close" @click="selectedICAO = ''" title="close">×</button>
        <div class="card-header">
          <span class="card-callsign">{{ selectedAircraft.callsign || '----' }}</span>
          <span class="card-icao">{{ selectedAircraft.icao }}</span>
        </div>
        <div class="card-grid">
          <div class="card-field">
            <span class="card-label">{{ t('adsb.altitude') }}</span>
            <span class="card-value">{{ selectedAircraft.altitude > 0 ? selectedAircraft.altitude + ' ft' : '---' }}</span>
          </div>
          <div class="card-field">
            <span class="card-label">{{ t('adsb.speed') }}</span>
            <span class="card-value">{{ selectedAircraft.speed > 0 ? selectedAircraft.speed.toFixed(0) + ' kt' : '---' }}</span>
          </div>
          <div class="card-field">
            <span class="card-label">{{ t('adsb.track') }}</span>
            <span class="card-value">{{ selectedAircraft.speed > 0 ? selectedAircraft.track.toFixed(0) + '°' : '---' }}</span>
          </div>
          <div class="card-field">
            <span class="card-label">{{ t('adsb.vRate') }}</span>
            <span class="card-value">{{ selectedAircraft.verticalRate !== 0 ? selectedAircraft.verticalRate + ' fpm' : '---' }}</span>
          </div>
          <div class="card-field">
            <span class="card-label">{{ t('adsb.lat') }}</span>
            <span class="card-value">{{ selectedAircraft.latitude.toFixed(4) }}°</span>
          </div>
          <div class="card-field">
            <span class="card-label">{{ t('adsb.lon') }}</span>
            <span class="card-value">{{ selectedAircraft.longitude.toFixed(4) }}°</span>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, computed } from 'vue'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { useAircraft, type Aircraft } from '../composables/useAircraft'
import { useStatus } from '../composables/useStatus'
import { useI18n } from '../composables/useI18n'

const { aircraft, selectedICAO } = useAircraft()
const { status } = useStatus()
const { t } = useI18n()

const mapContainer = ref<HTMLElement | null>(null)
let map: L.Map | null = null
let tileLayer: L.TileLayer | null = null
const markers: Map<string, L.Marker> = new Map()
// Last-applied per-marker state, so updateMarkers can skip DOM work
// (icon rebuild, popup content, position) for aircraft that haven't
// actually changed since the last WebSocket message.
interface MarkerState {
  lat: number
  lon: number
  track: number
  selected: boolean
}
const markerState: Map<string, MarkerState> = new Map()

// Default: Pacific Ocean, world view (no receiver position yet)
const DEFAULT_CENTER: L.LatLngExpression = [0, 160]
const DEFAULT_ZOOM = 2

function createTileLayer(): L.TileLayer {
  return L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    subdomains: ['a', 'b', 'c'],
    attribution: '© OpenStreetMap contributors',
    maxZoom: 18,
    keepBuffer: 4,
    crossOrigin: true,
    errorTileUrl: 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7',
  })
}

// Computed receiver position from status — changes when backend updates
const rxPos = computed(() => ({
  lat: status.value.RxLat,
  lon: status.value.RxLon,
}))

let lastRxLat = 0
let lastRxLon = 0

// The currently selected aircraft object (live-updating), or null.
const selectedAircraft = computed<Aircraft | null>(() => {
  if (!selectedICAO.value) return null
  return aircraft.value.find((a) => a.icao === selectedICAO.value) ?? null
})

function initMap() {
  if (!mapContainer.value || map) return

  // Start at receiver position if already known, otherwise Pacific Ocean world view
  const hasPos = rxPos.value.lat !== 0 && rxPos.value.lon !== 0
  const center = hasPos ? [rxPos.value.lat, rxPos.value.lon] as L.LatLngExpression : DEFAULT_CENTER
  const zoom = hasPos ? 8 : DEFAULT_ZOOM
  lastRxLat = rxPos.value.lat
  lastRxLon = rxPos.value.lon

  map = L.map(mapContainer.value, {
    center,
    zoom,
    zoomControl: true,
  })

  tileLayer = createTileLayer()
  tileLayer.addTo(map)
}

// Re-center map when receiver position changes
watch(rxPos, (pos) => {
  if (!map) return
  if (pos.lat === 0 && pos.lon === 0) return
  if (pos.lat === lastRxLat && pos.lon === lastRxLon) return
  lastRxLat = pos.lat
  lastRxLon = pos.lon
  map.flyTo([pos.lat, pos.lon], 8, { duration: 1.0 })
})

function buildIconHtml(track: number, selected: boolean): string {
  const color = selected ? '#ff8800' : '#00ff88'
  const stroke = selected ? '#442200' : '#004400'
  const size = selected ? 24 : 20
  const ring = selected
    ? `<div class="ac-ring"></div>`
    : ''
  return `${ring}<div class="ac-plane" style="transform: rotate(${track}deg);">
    <svg viewBox="0 0 24 24" width="${size}" height="${size}" style="display:block;">
      <path d="M12 2 L9.5 13 L2 16 L2 17.5 L9.5 15.5 L9.5 21 L12 20 L14.5 21 L14.5 15.5 L22 17.5 L22 16 L14.5 13 Z" fill="${color}" stroke="${stroke}" stroke-width="0.5"/>
    </svg>
  </div>`
}

function updateMarkers() {
  if (!map) return

  const seen = new Set<string>()

  for (const ac of aircraft.value) {
    if (ac.latitude === 0 && ac.longitude === 0) continue

    seen.add(ac.icao)
    const pos: L.LatLngExpression = [ac.latitude, ac.longitude]
    const selected = ac.icao === selectedICAO.value
    const hasVel = ac.speed > 0
    const rotDeg = hasVel ? ac.track : 0

    // Create or update marker
    let marker = markers.get(ac.icao)
    const prev = markerState.get(ac.icao)
    if (!marker) {
      const icon = L.divIcon({
        className: 'aircraft-marker',
        html: buildIconHtml(rotDeg, selected),
        iconSize: [24, 24],
        iconAnchor: [12, 12],
      })
      marker = L.marker(pos, { icon }).addTo(map!)
      // Click a marker to select that aircraft (toggle off if already selected).
      // Selection details are shown in the top-right info card, so no popup.
      marker.on('click', () => {
        selectedICAO.value = selectedICAO.value === ac.icao ? '' : ac.icao
      })
      markers.set(ac.icao, marker)
    } else {
      // Only touch the DOM for what actually changed — rebuilding the
      // divIcon (a DOM element) on every WS tick for every aircraft is
      // the expensive part, so it's gated on track/selection changing.
      if (prev && (prev.lat !== ac.latitude || prev.lon !== ac.longitude)) {
        marker.setLatLng(pos)
      }
      if (!prev || prev.track !== rotDeg || prev.selected !== selected) {
        marker.setIcon(L.divIcon({
          className: 'aircraft-marker',
          html: buildIconHtml(rotDeg, selected),
          iconSize: [24, 24],
          iconAnchor: [12, 12],
        }))
      }
    }

    markerState.set(ac.icao, { lat: ac.latitude, lon: ac.longitude, track: rotDeg, selected })
  }

  // Remove markers for aircraft no longer tracked
  for (const [icao, marker] of markers) {
    if (!seen.has(icao)) {
      map!.removeLayer(marker)
      markers.delete(icao)
      markerState.delete(icao)
    }
  }
}

// The aircraft array is replaced wholesale on every WebSocket message, so a
// shallow watch suffices (deep was redundant and added per-field overhead).
watch(aircraft, () => {
  updateMarkers()
})

// Fly to the selected aircraft when selection changes.
watch(selectedICAO, (icao) => {
  if (!map || !icao) return
  const ac = aircraft.value.find((a) => a.icao === icao)
  if (!ac || (ac.latitude === 0 && ac.longitude === 0)) return
  // Keep current zoom level if already close in, otherwise zoom in.
  const curZoom = map.getZoom()
  const targetZoom = curZoom < 8 ? 8 : curZoom
  map.flyTo([ac.latitude, ac.longitude], targetZoom, { duration: 0.8 })
  updateMarkers()
})

onMounted(() => {
  initMap()
  setTimeout(() => map?.invalidateSize(), 100)
})

onUnmounted(() => {
  markers.clear()
  markerState.clear()
  map?.remove()
  map = null
})
</script>

<style scoped>
.aircraft-map-wrap {
  position: relative;
  width: 100%;
  height: 100%;
}

.aircraft-map {
  width: 100%;
  height: 100%;
  background: #1a1a2e;
}

/* Selected-aircraft info card overlay */
.aircraft-info-card {
  position: absolute;
  top: 12px;
  right: 12px;
  z-index: 1000;
  width: 220px;
  background: rgba(26, 26, 46, 0.95);
  border: 1px solid #3a3a4e;
  border-radius: 8px;
  padding: 12px 14px 10px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(4px);
  color: #e0e0e0;
}

.card-close {
  position: absolute;
  top: 4px;
  right: 8px;
  background: none;
  border: none;
  color: #888;
  font-size: 20px;
  line-height: 1;
  cursor: pointer;
  padding: 2px 4px;
}

.card-close:hover {
  color: #ff8800;
}

.card-header {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid #333;
}

.card-callsign {
  font-size: 16px;
  font-weight: 700;
  color: #ff8800;
}

.card-icao {
  font-size: 12px;
  color: #888;
  font-family: monospace;
}

.card-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px 12px;
}

.card-field {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.card-label {
  font-size: 10px;
  color: #888;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.card-value {
  font-size: 13px;
  color: #e0e0e0;
  font-variant-numeric: tabular-nums;
}

/* Card enter/leave transition */
.card-enter-active,
.card-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}

.card-enter-from,
.card-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

/* Dark theme for Leaflet UI elements */
:deep(.leaflet-container) {
  background: #1a1a2e;
}

/* Convert light OSM tiles to dark theme via CSS filter */
:deep(.leaflet-tile-pane) {
  filter: invert(1) hue-rotate(180deg) brightness(0.9) contrast(0.85) saturate(0.8);
}

:deep(.leaflet-control-zoom a) {
  background: #2a2a3e !important;
  color: #ccc !important;
  border-color: #444 !important;
}

:deep(.leaflet-control-zoom a:hover) {
  background: #3a3a4e !important;
}

:deep(.leaflet-control-attribution) {
  background: rgba(26, 26, 46, 0.8) !important;
  color: #666 !important;
}

:deep(.leaflet-control-attribution a) {
  color: #888 !important;
}
</style>

<style>
.aircraft-marker {
  background: transparent !important;
  border: none !important;
}

/* Plane icon wrapper (rotation applied here) */
.aircraft-marker .ac-plane {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  transition: transform 0.3s ease;
}

/* Highlight ring for the selected aircraft (static, non-animated) */
.aircraft-marker .ac-ring {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 30px;
  height: 30px;
  margin: -15px 0 0 -15px;
  border: 2px solid #ff8800;
  border-radius: 50%;
  opacity: 0.7;
  pointer-events: none;
}
</style>
