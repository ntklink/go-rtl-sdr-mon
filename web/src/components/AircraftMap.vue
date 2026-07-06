<template>
  <div ref="mapContainer" class="aircraft-map"></div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, computed } from 'vue'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { useAircraft, type Aircraft } from '../composables/useAircraft'
import { useStatus } from '../composables/useStatus'

const { aircraft } = useAircraft()
const { status } = useStatus()

const mapContainer = ref<HTMLElement | null>(null)
let map: L.Map | null = null
let tileLayer: L.TileLayer | null = null
const markers: Map<string, L.Marker> = new Map()

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
}, { deep: true })

function formatAlt(alt: number): string {
  if (alt >= 1000) return (alt / 1000).toFixed(1) + 'km'
  return alt + 'm'
}

function updateMarkers() {
  if (!map) return

  const seen = new Set<string>()

  for (const ac of aircraft.value) {
    if (ac.latitude === 0 && ac.longitude === 0) continue

    seen.add(ac.icao)
    const pos: L.LatLngExpression = [ac.latitude, ac.longitude]

    // Create popup content
    const callsign = ac.callsign || '---'
    const popupHtml = `
      <div style="font-size:13px;line-height:1.6;">
        <b>${callsign}</b><br>
        ICAO: ${ac.icao}<br>
        Alt: ${ac.altitude > 0 ? ac.altitude + ' ft' : '---'}<br>
        Spd: ${ac.speed > 0 ? ac.speed.toFixed(0) + ' kt' : '---'}<br>
        Trk: ${ac.track > 0 ? ac.track.toFixed(0) + '°' : '---'}<br>
        ${ac.verticalRate !== 0 ? 'V/S: ' + ac.verticalRate + ' ft/min' : ''}
      </div>`

    // Create or update marker
    let marker = markers.get(ac.icao)
    if (marker) {
      marker.setLatLng(pos)
      marker.setPopupContent(popupHtml)
    } else {
      // Use a plane icon or default marker
      const icon = L.divIcon({
        className: 'aircraft-marker',
        html: `<div style="transform: rotate(${ac.track}deg); font-size:18px; color:#00ff88;">✈</div>`,
        iconSize: [24, 24],
        iconAnchor: [12, 12],
      })
      marker = L.marker(pos, { icon }).addTo(map!)
      marker.bindPopup(popupHtml)
      markers.set(ac.icao, marker)
    }

    // Update rotation if icon exists
    if (ac.track > 0) {
      const el = marker.getElement()
      if (el) {
        const inner = el.querySelector('div')
        if (inner) {
          inner.style.transform = `rotate(${ac.track}deg)`
        }
      }
    }
  }

  // Remove markers for aircraft no longer tracked
  for (const [icao, marker] of markers) {
    if (!seen.has(icao)) {
      map!.removeLayer(marker)
      markers.delete(icao)
    }
  }
}

watch(aircraft, () => {
  updateMarkers()
}, { deep: true })

onMounted(() => {
  initMap()
  setTimeout(() => map?.invalidateSize(), 100)
})

onUnmounted(() => {
  markers.clear()
  map?.remove()
  map = null
})
</script>

<style scoped>
.aircraft-map {
  width: 100%;
  height: 100%;
  background: #1a1a2e;
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
</style>
