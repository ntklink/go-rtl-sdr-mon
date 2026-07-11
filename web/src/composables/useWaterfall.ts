import { ref, onUnmounted, watch } from 'vue'

// Shared spectrum bins setting (number of bins sent over WebSocket)
// Default 1024 to reduce bandwidth; 0 = full FFT data
const spectrumBins = ref(1024)

// Export spectrumBins directly so other components (e.g. GainPanel)
// can access it without creating a redundant WebSocket connection.
export { spectrumBins }

// Colormap: muted viridis-inspired (dark navy → teal → green → warm yellow).
// Module-level so the LUT below can use it without per-call branching.
function getColor(norm: number): [number, number, number] {
  // Map dB range [-100, 0] to [0, 1]
  if (norm < 0.25) {
    const t = norm / 0.25
    return [
      Math.floor(t * 15),
      Math.floor(t * 25),
      Math.floor(20 + t * 70),
    ] // black → dark navy
  } else if (norm < 0.5) {
    const t = (norm - 0.25) / 0.25
    return [
      Math.floor(15 + t * 25),
      Math.floor(25 + t * 80),
      Math.floor(90 + t * 30),
    ] // dark navy → steel blue/teal
  } else if (norm < 0.75) {
    const t = (norm - 0.5) / 0.25
    return [
      Math.floor(40 + t * 80),
      Math.floor(105 + t * 75),
      Math.floor(120 - t * 60),
    ] // teal → green
  } else {
    const t = (norm - 0.75) / 0.25
    return [
      Math.floor(120 + t * 100),
      Math.floor(180 + t * 40),
      Math.floor(60 - t * 40),
    ] // green → warm yellow
  }
}

// Precomputed 256-entry colormap lookup table (maps a normalized 0..255
// brightness index to RGBA). Avoids re-running the branchy getColor for every
// pixel of every waterfall row.
const colormapLUT: Uint8ClampedArray = (() => {
  const lut = new Uint8ClampedArray(256 * 4)
  for (let i = 0; i < 256; i++) {
    const [r, g, b] = getColor(i / 255)
    lut[i * 4] = r
    lut[i * 4 + 1] = g
    lut[i * 4 + 2] = b
    lut[i * 4 + 3] = 255
  }
  return lut
})()

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
  // Set when a new FFT frame arrives, cleared after drawSpectrum runs, so
  // the rAF loop skips redrawing identical frames when the FFT rate (often
  // configured well below 60fps) is slower than the display refresh rate.
  let dirty = false
  // Last-drawn canvas dimensions. A resize updates the canvas width/height
  // attributes (clearing the bitmap), so the loop must also redraw on a
  // dimension change — otherwise the spectrum stays blank until the next
  // FFT frame, which at low FFT rates (1fps) is a visible gap.
  let lastW = 0
  let lastH = 0

  function setCanvases(spectrum: HTMLCanvasElement, waterfall: HTMLCanvasElement) {
    canvas = spectrum
    ctx = spectrum.getContext('2d')
    waterfallCanvas = waterfall
    waterfallCtx = waterfall.getContext('2d')
  }

  // Redraw only the spectrum trace (cheap). Runs every animation frame.
  function drawSpectrum() {
    if (!ctx || !fftData.value) return

    const w = canvas!.width
    const h = canvas!.height
    const data = fftData.value
    const n = data.length

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
    ctx.lineWidth = 0.5
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
  }

  // Advance the waterfall by exactly one row for the given FFT frame.
  // Called once per received FFT message (not per animation frame) so the
  // scroll rate tracks the FFT rate instead of the display refresh rate.
  function drawWaterfallRow(data: Float32Array) {
    if (!waterfallCtx || !waterfallCanvas) return
    const ww = waterfallCanvas.width

    // Scroll existing content down by 1px. drawImage of a canvas onto itself
    // with a pure translation is well-defined (the source bitmap is snapshotted
    // first) and is GPU-accelerated — far cheaper than getImageData/putImageData
    // of the whole canvas every frame.
    waterfallCtx.drawImage(waterfallCanvas, 0, 1)

    // Draw the new top row via the precomputed colormap LUT.
    const row = waterfallCtx.createImageData(ww, 1)
    const n = data.length
    const px = row.data
    for (let x = 0; x < ww; x++) {
      const idx = Math.floor((x / ww) * n)
      let norm = (data[idx] + 100) / 100
      if (norm < 0) norm = 0
      else if (norm > 1) norm = 1
      const ci = (norm * 255) | 0
      const o = x * 4
      const co = ci * 4
      px[o] = colormapLUT[co]
      px[o + 1] = colormapLUT[co + 1]
      px[o + 2] = colormapLUT[co + 2]
      px[o + 3] = 255
    }
    waterfallCtx.putImageData(row, 0, 0)
  }

  function loop() {
    rafId = requestAnimationFrame(loop)
    const resized = canvas !== null && (canvas.width !== lastW || canvas.height !== lastH)
    if (!dirty && !resized) return
    dirty = false
    if (canvas) {
      lastW = canvas.width
      lastH = canvas.height
    }
    drawSpectrum()
  }

  function connect() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const bins = spectrumBins.value
    const binsParam = bins > 0 ? `?bins=${bins}` : ''
    ws = new WebSocket(`${proto}//${location.host}/api/ws/fft${binsParam}`)
    ws.binaryType = 'arraybuffer'

    let headerReceived = false

    ws.onmessage = (ev) => {
      if (typeof ev.data === 'string') return

      const buf = new DataView(ev.data as ArrayBuffer)

      if (!headerReceived) {
        // First message: 4 bytes FFT size + 4 bytes output bins
        fftSize.value = buf.getUint32(0, true)
        headerReceived = true
        return
      }

      // FFT data: float32 values
      const n = buf.byteLength / 4
      const arr = new Float32Array(n)
      for (let i = 0; i < n; i++) {
        arr[i] = buf.getFloat32(i * 4, true)
      }
      fftData.value = arr
      dirty = true
      // Advance the waterfall one row per FFT frame.
      drawWaterfallRow(arr)
    }

    ws.onclose = () => {
      reconnectTimer = window.setTimeout(() => connect(), 2000)
    }
    ws.onerror = () => {
      ws?.close()
    }
  }

  // Reconnect when spectrum bins changes (replaces the 200ms polling loop)
  function onBinsChange() {
    if (ws) {
      ws.onclose = null
      ws.close()
      ws = null
    }
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    connect()
  }

  const stopWatch = watch(spectrumBins, onBinsChange)

  connect()
  loop()

  onUnmounted(() => {
    if (reconnectTimer) clearTimeout(reconnectTimer)
    if (rafId) cancelAnimationFrame(rafId)
    stopWatch()
    if (ws) {
      ws.onclose = null
      ws.onerror = null
      ws.close()
      ws = null
    }
  })

  return { fftSize, fftData, setCanvases, spectrumBins }
}
