import { ref, onUnmounted } from 'vue'

export interface ReceiverStatus {
  CenterFreq: number
  SampleRate: number
  SignalLevel: number
  SquelchOpen: boolean
  Demod: string
  FilterLow: number
  FilterHigh: number
  FilterOffset: number
}

export function useStatus() {
  const status = ref<ReceiverStatus>({
    CenterFreq: 100000000,
    SampleRate: 2400000,
    SignalLevel: -150,
    SquelchOpen: false,
    Demod: 'NFM',
    FilterLow: -5000,
    FilterHigh: 5000,
    FilterOffset: 0,
  })

  let ws: WebSocket | null = null
  let reconnectTimer: number | null = null

  function connect() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    ws = new WebSocket(`${proto}//${location.host}/api/ws/status`)
    ws.onmessage = (ev) => {
      try {
        const data = JSON.parse(ev.data)
        status.value = data
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

  connect()

  onUnmounted(() => {
    if (reconnectTimer) clearTimeout(reconnectTimer)
    ws?.close()
  })

  return { status }
}
