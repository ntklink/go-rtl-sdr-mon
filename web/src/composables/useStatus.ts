import { ref } from 'vue'

export interface ReceiverStatus {
  CenterFreq: number
  SampleRate: number
  SignalLevel: number
  SquelchOpen: boolean
  Demod: string
  FilterLow: number
  FilterHigh: number
  FilterOffset: number
  CWOffset: number
  FilterShape: string

  // Settings (synced from backend for state recovery)
  SquelchLevel: number
  SpectrumAvg: number
  FFTRate: number
  FFTMaxHold: boolean
  FFTSize: number
  AutoGain: boolean
  Gain: number
  FreqCorrection: number
  AGCOn: boolean
  AGCPreset: string

  // Receiver position (for ADS-B CPR decoding)
  RxLat: number
  RxLon: number
}

// Singleton state (shared across all callers)
const status = ref<ReceiverStatus>({
  CenterFreq: 100000000,
  SampleRate: 2400000,
  SignalLevel: -150,
  SquelchOpen: false,
  Demod: 'NFM',
  FilterLow: -5000,
  FilterHigh: 5000,
  FilterOffset: 0,
  CWOffset: 700,
  FilterShape: 'Normal',
  SquelchLevel: -150,
  SpectrumAvg: 0.3,
  FFTRate: 25,
  FFTMaxHold: false,
  FFTSize: 8192,
  AutoGain: true,
  Gain: 0,
  FreqCorrection: 0,
  AGCOn: true,
  AGCPreset: 'Medium',
  RxLat: 0,
  RxLon: 0,
})

let ws: WebSocket | null = null
let reconnectTimer: number | null = null
let connected = false

// True once the first real status has been received from the backend
export const statusLoaded = ref(false)

function connect() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  ws = new WebSocket(`${proto}//${location.host}/api/ws/status`)
  ws.onmessage = (ev) => {
    try {
      const data = JSON.parse(ev.data)
      status.value = data
      statusLoaded.value = true
    } catch (e) {
      // ignore parse errors
    }
  }
  ws.onclose = () => {
    reconnectTimer = window.setTimeout(() => connect(), 2000)
  }
  ws.onerror = () => {
    ws?.close()
  }
}

export function useStatus() {
  // Connect on first caller (singleton WebSocket)
  if (!connected) {
    connected = true
    connect()
  }

  return { status, statusLoaded }
}
