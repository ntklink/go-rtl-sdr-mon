<template>
  <div ref="mapContainer" class="aircraft-map"></div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { useAircraft, type Aircraft } from '../composables/useAircraft'
import { useI18n } from '../composables/useI18n'

const { aircraft } = useAircraft()
const { t, locale } = useI18n()

const mapContainer = ref<HTMLElement | null>(null)
let map: L.Map | null = null
let tileLayer: L.TileLayer | null = null
const markers: Map<string, L.Marker> = new Map()

function createTileLayer(lc: string): L.TileLayer {
  if (lc === 'zh-CN') {
    return L.tileLayer('https://webrd0{s}.is.autonavi.com/appmaptile?lang=zh_cn&size=1&scale=1&style=8&x={x}&y={y}&z={z}', {
      subdomains: ['1', '2', '3', '4'],
      attribution: '© AutoNavi',
      maxZoom: 18,
    })
  }
  return L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    attribution: '© OpenStreetMap',
    maxZoom: 18,
  })
}

function initMap() {
  if (!mapContainer.value || map) return

  map = L.map(mapContainer.value, {
    center: [39.9, 116.4],
    zoom: 7,
    zoomControl: true,
  })

  tileLayer = createTileLayer(locale.value)
  tileLayer.addTo(map)
}

// Switch tile layer when locale changes
watch(locale, (lc) => {
  if (!map) return
  if (tileLayer) {
    map.removeLayer(tileLayer)
  }
  tileLayer = createTileLayer(lc)
  tileLayer.addTo(map)
})

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
</style>

<style>
.aircraft-marker {
  background: transparent !important;
  border: none !important;
}
</style>
