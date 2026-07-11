import { ref, watch, onUnmounted } from 'vue'
import { useStatus } from './useStatus'
import { useApi } from './useApi'

export interface APTLine {
  lineNum: number
  pixels: Uint8Array // 2080 bytes
}

export interface Satellite {
  name: string
  frequency: number
  period: number
  status: string
}

const APT_LINE_WIDTH = 2080

// Shared state across components
const lines = ref<Uint8Array[]>([])
const maxLines = 2000
// Monotonically increasing counter of received lines (incremented even when
// the buffer is full and a shift occurs), so the UI can detect new lines
// without polling and redraw incrementally.
const receivedLines = ref(0)
const stats = ref<{ lines: number; sync: number; signalLevel: number }>({ lines: 0, sync: 0, signalLevel: 0 })
let ws: WebSocket | null = null
let statsTimer: ReturnType<typeof setInterval> | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
// Set by disconnectAPT() right before closing, so onclose can tell an
// intentional stop (leaving NOAA mode, unmount) apart from an unexpected
// drop that should trigger a reconnect.
let intentionalClose = false
let refCount = 0

const api = useApi()

function connectAPT() {
  if (ws) return
  intentionalClose = false

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const wsURL = `${protocol}//${window.location.host}/api/ws/apt`

  ws = new WebSocket(wsURL)
  ws.binaryType = 'arraybuffer'

  ws.onmessage = (event) => {
    const ab = event.data as ArrayBuffer
    if (ab.byteLength < 4) return

    // The 4-byte line number prefix is unused on the client (display follows
    // arrival order, which equals line order over an ordered WebSocket).
    // Zero-copy view of the pixel bytes (the WebSocket buffer is not reused).
    const pixels = new Uint8Array(ab, 4)

    // Append in arrival order and cap to maxLines. A simple shift keeps the
    // array in display order (the previous lineNum%-based ring scrambled
    // rows after wrapping past maxLines).
    lines.value.push(pixels)
    if (lines.value.length > maxLines) {
      lines.value.shift()
    }
    receivedLines.value++
  }

  ws.onclose = () => {
    ws = null
    if (!intentionalClose) {
      reconnectTimer = setTimeout(() => connectAPT(), 2000)
    }
  }

  ws.onerror = () => {
    ws?.close()
  }
}

function disconnectAPT() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  if (ws) {
    intentionalClose = true
    ws.close()
    ws = null
  }
}

function startStatsPolling() {
  if (statsTimer) return
  const poll = async () => {
    try {
      stats.value = await api.getAPTStats()
    } catch {
      // ignore
    }
  }
  poll()
  statsTimer = setInterval(poll, 2000)
}

function stopStatsPolling() {
  if (statsTimer) {
    clearInterval(statsTimer)
    statsTimer = null
  }
}

export function useNoAA() {
  const { status } = useStatus()
  refCount++

  // Auto-connect when in NOAA mode
  if (status.value.Demod === 'NOAA') {
    connectAPT()
    startStatsPolling()
  }

  // Watch for demod changes
  const stopWatch = watch(() => status.value.Demod, (demod) => {
    if (demod === 'NOAA') {
      connectAPT()
      startStatsPolling()
    } else {
      disconnectAPT()
      stopStatsPolling()
    }
  })

  function resetImage() {
    lines.value = []
    receivedLines.value = 0
    api.resetAPT()
  }

  function saveImage() {
    if (lines.value.length === 0) return

    // Create canvas and draw the image
    const canvas = document.createElement('canvas')
    canvas.width = APT_LINE_WIDTH
    canvas.height = lines.value.length
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const imageData = ctx.createImageData(canvas.width, canvas.height)
    for (let y = 0; y < lines.value.length; y++) {
      const line = lines.value[y]
      for (let x = 0; x < APT_LINE_WIDTH && x < line.length; x++) {
        const v = line[x]
        const idx = (y * canvas.width + x) * 4
        imageData.data[idx] = v
        imageData.data[idx + 1] = v
        imageData.data[idx + 2] = v
        imageData.data[idx + 3] = 255
      }
    }
    ctx.putImageData(imageData, 0, 0)

    // Download as PNG
    canvas.toBlob((blob) => {
      if (!blob) return
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `noaa-apt-${Date.now()}.png`
      a.click()
      URL.revokeObjectURL(url)
    })
  }

  onUnmounted(() => {
    refCount--
    if (refCount <= 0) {
      disconnectAPT()
      stopStatsPolling()
      refCount = 0
    }
    stopWatch()
  })

  return {
    lines,
    receivedLines,
    stats,
    resetImage,
    saveImage,
  }
}
