import { ref, onUnmounted } from 'vue'

export interface Aircraft {
  icao: string
  callsign: string
  latitude: number
  longitude: number
  altitude: number
  speed: number
  track: number
  verticalRate: number
  squawk: string
  onGround: boolean
  typeCode: number
  lastSeen: number
  messageCount: number
}

let ws: WebSocket | null = null
let reconnectTimer: number | null = null
let refCount = 0

const aircraft = ref<Aircraft[]>([])
export const aircraftLoaded = ref(false)

// Shared selected aircraft ICAO address — set by AircraftPanel row clicks
// or AircraftMap marker clicks, consumed by both for highlighting.
export const selectedICAO = ref('')

// History mode: when true, the UI shows all aircraft ever seen.
export const showHistory = ref(false)
const historyAircraft = ref<Aircraft[]>([])

function connect() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  ws = new WebSocket(`${proto}//${location.host}/api/ws/aircraft`)
  ws.onmessage = (ev) => {
    try {
      const data = JSON.parse(ev.data)
      if (Array.isArray(data)) {
        aircraft.value = data
        aircraftLoaded.value = true
        // If in history mode, also refresh history list so timestamps
        // stay current.  The history list is polled via REST.
      }
    } catch (e) {
      // ignore
    }
  }
  ws.onclose = () => {
    reconnectTimer = window.setTimeout(() => connect(), 2000)
  }
  ws.onerror = () => {
    ws?.close()
  }
}

export function useAircraft() {
  if (refCount === 0) {
    connect()
  }
  refCount++

  onUnmounted(() => {
    refCount--
    if (refCount <= 0) {
      refCount = 0
      if (reconnectTimer) clearTimeout(reconnectTimer)
      ws?.close()
      ws = null
    }
  })

  return { aircraft, aircraftLoaded, showHistory, historyAircraft, selectedICAO }
}
