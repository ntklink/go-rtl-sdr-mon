import { ref, watch, onUnmounted } from 'vue'
import { useStatus } from './useStatus'
import { useApi } from './useApi'

export interface Satellite {
  name: string
  frequency: number
  period: number
  status: string
}

export interface LRPTStats {
  locked: boolean
  signalQ: number
  freqOffset: number
  framesOK: number
  framesBad: number
  rsCorrected: number
  packets: number
  apids: number[] | null
}

export const LRPT_IMAGE_WIDTH = 1568
const SEG_WIDTH = 112 // 14 MCUs × 8 px
const SEG_HEIGHT = 8

// One offscreen canvas per APID; segments are painted as they arrive and
// the panel blits the selected channel to its visible canvas.
interface Channel {
  canvas: HTMLCanvasElement
  ctx: CanvasRenderingContext2D
  maxStrip: number
}

const channels = new Map<number, Channel>()
const apids = ref<number[]>([])
// Monotonic counter bumped on every received segment; the panel watches
// it and repaints incrementally.
const segCounter = ref(0)
// Last received segment's placement (for incremental blitting)
export interface SegPos {
  apid: number
  x: number
  y: number
  w: number
  h: number
}
let lastSeg: SegPos | null = null

const stats = ref<LRPTStats>({
  locked: false, signalQ: 0, freqOffset: 0,
  framesOK: 0, framesBad: 0, rsCorrected: 0, packets: 0, apids: [],
})
const constellation = ref<number[]>([])

let ws: WebSocket | null = null
let statsTimer: ReturnType<typeof setInterval> | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
let intentionalClose = false
let refCount = 0

const api = useApi()

function getChannel(apid: number): Channel {
  let ch = channels.get(apid)
  if (!ch) {
    const canvas = document.createElement('canvas')
    canvas.width = LRPT_IMAGE_WIDTH
    canvas.height = 512
    const ctx = canvas.getContext('2d')!
    ctx.fillStyle = '#000'
    ctx.fillRect(0, 0, canvas.width, canvas.height)
    ch = { canvas, ctx, maxStrip: -1 }
    channels.set(apid, ch)
    apids.value = [...channels.keys()].sort((a, b) => a - b)
  }
  return ch
}

function ensureHeight(ch: Channel, needed: number) {
  if (needed <= ch.canvas.height) return
  let h = ch.canvas.height
  while (h < needed) h *= 2
  const tmp = document.createElement('canvas')
  tmp.width = ch.canvas.width
  tmp.height = ch.canvas.height
  tmp.getContext('2d')!.drawImage(ch.canvas, 0, 0)
  ch.canvas.height = h
  ch.ctx.fillStyle = '#000'
  ch.ctx.fillRect(0, 0, ch.canvas.width, h)
  ch.ctx.drawImage(tmp, 0, 0)
}

function handleSegment(ab: ArrayBuffer) {
  if (ab.byteLength < 8 + SEG_WIDTH * SEG_HEIGHT) return
  const dv = new DataView(ab)
  const apid = dv.getUint16(0, true)
  const strip = dv.getUint32(2, true)
  const mcuIndex = dv.getUint8(6)
  const pixels = new Uint8Array(ab, 8)

  const ch = getChannel(apid)
  const y = strip * SEG_HEIGHT
  const x = mcuIndex * 8
  ensureHeight(ch, y + SEG_HEIGHT)

  const img = ch.ctx.createImageData(SEG_WIDTH, SEG_HEIGHT)
  for (let i = 0; i < SEG_WIDTH * SEG_HEIGHT; i++) {
    const v = pixels[i]
    img.data[i * 4] = v
    img.data[i * 4 + 1] = v
    img.data[i * 4 + 2] = v
    img.data[i * 4 + 3] = 255
  }
  ch.ctx.putImageData(img, x, y)
  if (strip > ch.maxStrip) ch.maxStrip = strip

  lastSeg = { apid, x, y, w: SEG_WIDTH, h: SEG_HEIGHT }
  segCounter.value++
}

function connect() {
  if (ws) return
  intentionalClose = false
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  ws = new WebSocket(`${protocol}//${window.location.host}/api/ws/lrpt`)
  ws.binaryType = 'arraybuffer'
  ws.onmessage = (event) => handleSegment(event.data as ArrayBuffer)
  ws.onclose = () => {
    ws = null
    if (!intentionalClose) {
      reconnectTimer = setTimeout(() => connect(), 2000)
    }
  }
  ws.onerror = () => ws?.close()
}

function disconnect() {
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
      const resp = await api.getLRPTStats()
      stats.value = resp.stats
      constellation.value = resp.constellation || []
    } catch {
      // ignore
    }
  }
  poll()
  statsTimer = setInterval(poll, 1000)
}

function stopStatsPolling() {
  if (statsTimer) {
    clearInterval(statsTimer)
    statsTimer = null
  }
}

export function useLRPT() {
  const { status } = useStatus()
  refCount++

  if (status.value.Demod === 'LRPT') {
    connect()
    startStatsPolling()
  }

  const stopWatch = watch(() => status.value.Demod, (demod) => {
    if (demod === 'LRPT') {
      connect()
      startStatsPolling()
    } else {
      disconnect()
      stopStatsPolling()
    }
  })

  function resetImage() {
    channels.clear()
    apids.value = []
    lastSeg = null
    segCounter.value++
    api.resetLRPT()
  }

  function saveImage(apid: number) {
    const ch = channels.get(apid)
    if (!ch || ch.maxStrip < 0) return
    // Export only the drawn region
    const out = document.createElement('canvas')
    out.width = LRPT_IMAGE_WIDTH
    out.height = (ch.maxStrip + 1) * SEG_HEIGHT
    out.getContext('2d')!.drawImage(ch.canvas, 0, 0)
    out.toBlob((blob) => {
      if (!blob) return
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `meteor-lrpt-apid${apid}-${Date.now()}.png`
      a.click()
      URL.revokeObjectURL(url)
    })
  }

  onUnmounted(() => {
    refCount--
    if (refCount <= 0) {
      disconnect()
      stopStatsPolling()
      refCount = 0
    }
    stopWatch()
  })

  return {
    apids,
    stats,
    constellation,
    segCounter,
    getChannelCanvas: (apid: number) => channels.get(apid)?.canvas ?? null,
    getChannelHeight: (apid: number) => {
      const ch = channels.get(apid)
      return ch && ch.maxStrip >= 0 ? (ch.maxStrip + 1) * SEG_HEIGHT : 0
    },
    lastSegment: () => lastSeg,
    resetImage,
    saveImage,
  }
}
