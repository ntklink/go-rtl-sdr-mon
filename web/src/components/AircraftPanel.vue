<template>
  <div class="panel adsb-panel">
    <h3>{{ t('adsb.aircraft') }} ({{ aircraft.length }})</h3>

    <div class="stats-bar" v-if="stats">
      <span class="stat-item">
        <span class="stat-label">{{ t('adsb.detected') }}:</span>
        <span class="stat-value" :class="{ zero: stats.detected === 0 }">{{ fmt(stats.detected) }}</span>
      </span>
      <span class="stat-item">
        <span class="stat-label">{{ t('adsb.valid') }}:</span>
        <span class="stat-value" :class="{ zero: stats.valid === 0 }">{{ fmt(stats.valid) }}</span>
      </span>
      <span class="stat-item">
        <span class="stat-label">{{ t('adsb.accepted') }}:</span>
        <span class="stat-value" :class="{ zero: stats.accepted === 0 }">{{ fmt(stats.accepted) }}</span>
      </span>
    </div>
    <div class="tip-bar" v-if="stats && stats.detected === 0">
      {{ t('adsb.tip') }}
    </div>

    <div class="history-toggle">
      <label class="toggle-label">
        <span class="toggle-track" :class="{ on: showHistory }">
          <span class="toggle-thumb"></span>
        </span>
        <input type="checkbox" v-model="showHistory" @change="onHistoryToggle" />
        <span class="toggle-text">{{ t('adsb.showHistory') }}</span>
      </label>
      <span class="history-count" v-if="showHistory">
        ({{ displayAircraft.length }})
      </span>
    </div>

    <div class="control-group">
      <label>{{ t('adsb.rxPos') }}</label>
      <div class="rx-pos-row">
        <input type="number" v-model.number="rxLat" class="input rx-input" :placeholder="t('adsb.lat')"
          step="0.00001" />
        <input type="number" v-model.number="rxLon" class="input rx-input" :placeholder="t('adsb.lon')"
          step="0.00001" />
        <button class="btn-geo" @click="requestGeolocation" :disabled="geoLoading" :title="t('adsb.geoLocate')">
          {{ geoLoading ? '…' : '📍' }}
        </button>
        <button class="btn-set" @click="setRxPos">{{ t('adsb.setRxPos') }}</button>
      </div>
      <div v-if="geoError" class="geo-error">{{ geoError }}</div>
    </div>

    <div v-if="displayAircraft.length === 0" class="no-data">
      {{ showHistory ? t('adsb.noHistory') : t('adsb.noData') }}
    </div>

    <div v-else class="aircraft-table">
      <div class="table-header">
        <span class="col-callsign">{{ t('adsb.callsign') }}</span>
        <span class="col-alt">{{ t('adsb.altitude') }}</span>
        <span class="col-spd">{{ t('adsb.speed') }}</span>
        <span class="col-trk">{{ t('adsb.track') }}</span>
      </div>
      <div v-for="ac in sortedAircraft" :key="ac.icao" class="table-row"
        :class="{ selected: selectedICAO === ac.icao, inactive: showHistory && !isActive(ac) }"
        @click="selectAircraft(ac)">
        <span class="col-callsign">
          <span class="callsign-text">{{ ac.callsign || '----' }}</span>
          <span class="icao-text">{{ ac.icao }}</span>
        </span>
        <span class="col-alt">{{ ac.altitude > 0 ? ac.altitude + ' ft' : '---' }}</span>
        <span class="col-spd">{{ ac.speed > 0 ? ac.speed.toFixed(0) + ' kt' : '---' }}</span>
        <span class="col-trk">{{ ac.track > 0 ? ac.track.toFixed(0) + '°' : '---' }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useAircraft, type Aircraft } from '../composables/useAircraft'
import { useApi } from '../composables/useApi'
import { useI18n } from '../composables/useI18n'
import { useStatus, statusLoaded } from '../composables/useStatus'

const { aircraft, showHistory, historyAircraft, selectedICAO } = useAircraft()
const api = useApi()
const { t } = useI18n()
const { status } = useStatus()

const rxLat = ref<number | undefined>(undefined)
const rxLon = ref<number | undefined>(undefined)
const geoLoading = ref(false)
const geoError = ref('')
const stats = ref<{ detected: number; valid: number; accepted: number; aircraft: number } | null>(null)
let statsTimer: ReturnType<typeof setInterval> | null = null

// Compact large numbers to avoid overflow in the stats bar (e.g. 12345 -> 12.3k).
function fmt(n: number): string {
  if (n < 1000) return String(n)
  if (n < 1_000_000) {
    const v = n / 1000
    return (v >= 100 ? v.toFixed(0) : v.toFixed(1).replace(/\.0$/, '')) + 'k'
  }
  const v = n / 1_000_000
  return (v >= 100 ? v.toFixed(0) : v.toFixed(1).replace(/\.0$/, '')) + 'M'
}

// The displayed list depends on whether history mode is toggled on.
const displayAircraft = computed(() => {
  return showHistory.value ? historyAircraft.value : aircraft.value
})

// Whether a given aircraft is currently active (seen in last 120 seconds).
function isActive(ac: Aircraft): boolean {
  if (!ac.lastSeen) return false
  return Date.now() - ac.lastSeen < 120_000
}

// When history mode is on, poll the history endpoint every few seconds
// to get fresh timestamps and any new aircraft.
async function pollHistory() {
  if (!showHistory.value) return
  try {
    const resp = await api.getAircraftHistory()
    if (resp && Array.isArray(resp.aircraft)) {
      historyAircraft.value = resp.aircraft
    }
  } catch {
    // ignore
  }
}

function onHistoryToggle() {
  if (showHistory.value) {
    pollHistory()
  }
}

async function pollStats() {
  try {
    const s = await api.getADSBStats()
    stats.value = s
  } catch {
    // ignore
  }
}

// Sync receiver position from backend status (e.g. after page refresh)
watch(statusLoaded, (loaded) => {
  if (loaded && status.value.RxLat !== 0 && status.value.RxLon !== 0) {
    rxLat.value = status.value.RxLat
    rxLon.value = status.value.RxLon
  }
}, { immediate: true })

function selectAircraft(ac: Aircraft) {
  selectedICAO.value = selectedICAO.value === ac.icao ? '' : ac.icao
}

const sortedAircraft = computed(() => {
  return [...displayAircraft.value].sort((a, b) => {
    // In history mode, sort by LastSeen descending (most recent first).
    if (showHistory.value) {
      return (b.lastSeen || 0) - (a.lastSeen || 0)
    }
    if (a.callsign && !b.callsign) return -1
    if (!a.callsign && b.callsign) return 1
    return a.callsign.localeCompare(b.callsign)
  })
})

async function setRxPos() {
  if (rxLat.value == null || rxLon.value == null) return
  try {
    await api.setReceiverPosition(rxLat.value, rxLon.value)
  } catch (e) {
    console.error('Set receiver position failed:', e)
  }
}

function requestGeolocation() {
  if (!navigator.geolocation) {
    geoError.value = t('adsb.geoUnsupported')
    return
  }
  geoLoading.value = true
  geoError.value = ''
  navigator.geolocation.getCurrentPosition(
    async (pos) => {
      geoLoading.value = false
      rxLat.value = parseFloat(pos.coords.latitude.toFixed(5))
      rxLon.value = parseFloat(pos.coords.longitude.toFixed(5))
      try {
        await api.setReceiverPosition(rxLat.value, rxLon.value)
      } catch (e) {
        console.error('Set receiver position failed:', e)
      }
    },
    (err) => {
      geoLoading.value = false
      if (err.code === err.PERMISSION_DENIED) {
        geoError.value = t('adsb.geoDenied')
      } else if (err.code === err.POSITION_UNAVAILABLE) {
        geoError.value = t('adsb.geoUnavailable')
      } else if (err.code === err.TIMEOUT) {
        geoError.value = t('adsb.geoTimeout')
      } else {
        geoError.value = err.message
      }
    },
    { enableHighAccuracy: true, timeout: 10000 }
  )
}

// Auto-request browser geolocation on mount if no position is set yet
onMounted(() => {
  if (status.value.RxLat === 0 && status.value.RxLon === 0) {
    requestGeolocation()
  } else {
    rxLat.value = status.value.RxLat
    rxLon.value = status.value.RxLon
  }
  // Poll ADS-B decoder stats every 2 seconds, and history if enabled
  pollStats()
  statsTimer = setInterval(() => {
    pollStats()
    if (showHistory.value) {
      pollHistory()
    }
  }, 2000)
})

onUnmounted(() => {
  if (statsTimer) {
    clearInterval(statsTimer)
    statsTimer = null
  }
})
</script>

<style scoped>
.adsb-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 100%;
  overflow: hidden;
}

.stats-bar {
  display: flex;
  gap: 12px;
  padding: 4px 8px;
  background: #1a1a2e;
  border-radius: 4px;
  font-size: 11px;
  overflow: hidden;
}

.stat-item {
  display: flex;
  gap: 4px;
  align-items: center;
  min-width: 0;
}

.stat-label {
  color: #888;
  white-space: nowrap;
}

.stat-value {
  color: #4a4;
  font-family: 'Courier New', monospace;
  font-weight: bold;
  white-space: nowrap;
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.stat-value.zero {
  color: #a44;
}

.tip-bar {
  font-size: 10px;
  color: #aa0;
  padding: 4px 8px;
  background: #2a2a10;
  border-radius: 4px;
  line-height: 1.4;
}

.history-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 2px 0;
  font-size: 12px;
}

.toggle-label {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  color: #aaa;
  position: relative;
}

/* Hide native checkbox */
.toggle-label input[type="checkbox"] {
  position: absolute;
  opacity: 0;
  width: 0;
  height: 0;
  pointer-events: none;
}

/* iOS-style toggle switch */
.toggle-track {
  display: inline-block;
  position: relative;
  width: 32px;
  height: 18px;
  border-radius: 9px;
  background: #444;
  transition: background 0.2s ease;
  flex-shrink: 0;
}

.toggle-track.on {
  background: #0c7;
}

.toggle-thumb {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: #fff;
  transition: transform 0.2s ease;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
}

.toggle-track.on .toggle-thumb {
  transform: translateX(14px);
}

.toggle-text {
  user-select: none;
}

.history-count {
  color: #666;
  font-size: 11px;
}

.table-row.inactive {
  opacity: 0.5;
}

.rx-pos-row {
  display: flex;
  gap: 4px;
}

.rx-input {
  flex: 1;
  min-width: 0;
}

.btn-set {
  padding: 4px 8px;
  background: #0066cc;
  border: none;
  color: #fff;
  border-radius: 3px;
  font-size: 11px;
  cursor: pointer;
  flex-shrink: 0;
}

.btn-set:hover {
  background: #0052a3;
}

.btn-geo {
  padding: 4px 8px;
  background: #333;
  border: none;
  color: #fff;
  border-radius: 3px;
  font-size: 13px;
  cursor: pointer;
  flex-shrink: 0;
  line-height: 1;
}

.btn-geo:hover:not(:disabled) {
  background: #444;
}

.btn-geo:disabled {
  opacity: 0.5;
  cursor: wait;
}

.geo-error {
  font-size: 10px;
  color: #ff6666;
  padding: 2px 0;
}

.no-data {
  color: #666;
  font-size: 12px;
  text-align: center;
  padding: 20px 0;
}

.aircraft-table {
  flex: 1;
  overflow-y: auto;
  border: 1px solid #222;
  border-radius: 4px;
}

.table-header {
  display: flex;
  padding: 6px 8px;
  background: #1a1a2e;
  border-bottom: 1px solid #333;
  font-size: 10px;
  color: #888;
  text-transform: uppercase;
  position: sticky;
  top: 0;
  z-index: 1;
}

.table-row {
  display: flex;
  padding: 5px 8px;
  border-bottom: 1px solid #1a1a1a;
  cursor: pointer;
  transition: background 0.1s;
}

.table-row:hover {
  background: #1a1a2e;
}

.table-row.selected {
  background: #1a2a4e;
}

.col-callsign {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.callsign-text {
  font-size: 12px;
  color: #fff;
  font-family: 'Courier New', monospace;
}

.icao-text {
  font-size: 9px;
  color: #555;
}

.col-alt {
  width: 60px;
  text-align: right;
  font-size: 11px;
  color: #aaa;
  font-family: 'Courier New', monospace;
}

.col-spd {
  width: 50px;
  text-align: right;
  font-size: 11px;
  color: #aaa;
  font-family: 'Courier New', monospace;
}

.col-trk {
  width: 40px;
  text-align: right;
  font-size: 11px;
  color: #aaa;
  font-family: 'Courier New', monospace;
}
</style>
