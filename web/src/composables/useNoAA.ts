import { ref, onUnmounted } from 'vue'
import { useStatus } from './useStatus'

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
const stats = ref<{ lines: number; sync: number }>({ lines: 0, sync: 0 })
let ws: WebSocket | null = null
let statsTimer: ReturnType<typeof setInterval> | null = null
let refCount = 0

function connectAPT() {
  if (ws) return

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const wsURL = `${protocol}//${window.location.host}/api/ws/apt`

  ws = new WebSocket(wsURL)
  ws.binaryType = 'arraybuffer'

  ws.onmessage = (event) => {
    const data = new Uint8Array(event.data)
    if (data.length < 4) return

    const view = new DataView(event.data)
    const lineNum = view.getUint32(0, true)
    const pixels = data.slice(4)

    // Store line (circular buffer)
    if (lines.value.length >= maxLines) {
      const idx = lineNum % maxLines
      if (idx < lines.value.length) {
        lines.value[idx] = pixels
      }
    } else {
      lines.value.push(pixels)
    }
  }

  ws.onclose = () => {
    ws = null
  }

  ws.onerror = () => {
    // Error handling - will reconnect if still needed
  }
}

function disconnectAPT() {
  if (ws) {
    ws.close()
    ws = null
  }
}

function startStatsPolling() {
  if (statsTimer) return
  const poll = async () => {
    try {
      const resp = await fetch('/api/apt-stats')
      if (resp.ok) {
        stats.value = await resp.json()
      }
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
  import('vue').then(({ watch }) => {
    watch(() => status.value.Demod, (demod) => {
      if (demod === 'NOAA') {
        connectAPT()
        startStatsPolling()
      } else {
        disconnectAPT()
        stopStatsPolling()
      }
    })
  })

  function resetImage() {
    lines.value = []
    fetch('/api/apt-reset', { method: 'POST' })
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
  })

  return {
    lines,
    stats,
    resetImage,
    saveImage,
  }
}
