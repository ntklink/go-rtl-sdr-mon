import { ref, onUnmounted } from 'vue'

export function useWaterfall() {
  const fftSize = ref(2048)
  const fftData = ref<Float32Array | null>(null)

  let ws: WebSocket | null = null
  let canvas: HTMLCanvasElement | null = null
  let ctx: CanvasRenderingContext2D | null = null
  let waterfallCanvas: HTMLCanvasElement | null = null
  let waterfallCtx: CanvasRenderingContext2D | null = null
  let reconnectTimer: number | null = null
  let rafId: number | null = null

  // Colormap: blue -> cyan -> green -> yellow -> red
  function getColor(db: number): [number, number, number] {
    // Map dB range [-100, 0] to [0, 1]
    const norm = Math.max(0, Math.min(1, (db + 100) / 100))
    if (norm < 0.2) {
      const t = norm / 0.2
      return [0, 0, Math.floor(t * 255)]
    } else if (norm < 0.4) {
      const t = (norm - 0.2) / 0.2
      return [0, Math.floor(t * 255), 255]
    } else if (norm < 0.6) {
      const t = (norm - 0.4) / 0.2
      return [0, 255, Math.floor(255 - t * 255)]
    } else if (norm < 0.8) {
      const t = (norm - 0.6) / 0.2
      return [Math.floor(t * 255), 255, 0]
    } else {
      const t = (norm - 0.8) / 0.2
      return [255, Math.floor(255 - t * 255), 0]
    }
  }

  function setCanvases(spectrum: HTMLCanvasElement, waterfall: HTMLCanvasElement) {
    canvas = spectrum
    ctx = spectrum.getContext('2d')
    waterfallCanvas = waterfall
    waterfallCtx = waterfall.getContext('2d')
    draw()
  }

  function draw() {
    rafId = requestAnimationFrame(draw)
    if (!ctx || !fftData.value) return

    const w = canvas!.width
    const h = canvas!.height
    const data = fftData.value
    const n = data.length

    // Draw spectrum
    ctx.fillStyle = '#0a0a0a'
    ctx.fillRect(0, 0, w, h)

    // Grid
    ctx.strokeStyle = '#1a1a2e'
    ctx.lineWidth = 1
    for (let i = 0; i <= 10; i++) {
      const x = (i / 10) * w
      ctx.beginPath()
      ctx.moveTo(x, 0)
      ctx.lineTo(x, h)
      ctx.stroke()
    }
    for (let i = 0; i <= 8; i++) {
      const y = (i / 8) * h
      ctx.beginPath()
      ctx.moveTo(0, y)
      ctx.lineTo(w, y)
      ctx.stroke()
    }

    // Spectrum line
    ctx.strokeStyle = '#00ff88'
    ctx.lineWidth = 1
    ctx.beginPath()
    for (let i = 0; i < n; i++) {
      const x = (i / (n - 1)) * w
      const db = data[i]
      const y = h - ((db + 100) / 100) * h
      const yClamped = Math.max(0, Math.min(h, y))
      if (i === 0) {
        ctx.moveTo(x, yClamped)
      } else {
        ctx.lineTo(x, yClamped)
      }
    }
    ctx.stroke()

    // Filled area
    ctx.lineTo(w, h)
    ctx.lineTo(0, h)
    ctx.closePath()
    ctx.fillStyle = 'rgba(0, 255, 136, 0.08)'
    ctx.fill()

    // Waterfall: shift down by 1 pixel and draw new line at top
    if (waterfallCtx) {
      const ww = waterfallCanvas!.width
      const wh = waterfallCanvas!.height

      // Shift existing content down
      const imageData = waterfallCtx.getImageData(0, 0, ww, wh - 1)
      waterfallCtx.putImageData(imageData, 0, 1)

      // Draw new line at top
      const lineImageData = waterfallCtx.createImageData(ww, 1)
      for (let x = 0; x < ww; x++) {
        const idx = Math.floor((x / ww) * n)
        const [r, g, b] = getColor(data[idx])
        lineImageData.data[x * 4] = r
        lineImageData.data[x * 4 + 1] = g
        lineImageData.data[x * 4 + 2] = b
        lineImageData.data[x * 4 + 3] = 255
      }
      waterfallCtx.putImageData(lineImageData, 0, 0)
    }
  }

  function connect() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    ws = new WebSocket(`${proto}//${location.host}/api/ws/fft`)
    ws.binaryType = 'arraybuffer'

    let sizeReceived = false

    ws.onmessage = (ev) => {
      if (typeof ev.data === 'string') return

      const buf = new DataView(ev.data as ArrayBuffer)

      if (!sizeReceived) {
        // First message: 4 bytes = FFT size
        fftSize.value = buf.getUint32(0, true)
        sizeReceived = true
        return
      }

      // FFT data: float32 values
      const n = buf.byteLength / 4
      const arr = new Float32Array(n)
      for (let i = 0; i < n; i++) {
        arr[i] = buf.getFloat32(i * 4, true)
      }
      fftData.value = arr
    }

    ws.onclose = () => {
      reconnectTimer = window.setTimeout(() => connect(), 2000)
    }
    ws.onerror = () => {
      ws?.close()
    }
  }

  connect()
  draw()

  onUnmounted(() => {
    if (reconnectTimer) clearTimeout(reconnectTimer)
    if (rafId) cancelAnimationFrame(rafId)
    ws?.close()
  })

  return { fftSize, fftData, setCanvases }
}
