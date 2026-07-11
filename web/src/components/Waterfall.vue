<template>
  <div class="waterfall-container" ref="containerRef">
    <!-- Frequency axis -->
    <div class="freq-axis" ref="freqAxis">
      <span v-for="(tick, i) in freqTicks" :key="tick.freq"
        :class="{ 'tick-left': i === 0, 'tick-right': i === freqTicks.length - 1 }" :style="{ left: tick.pos + '%' }">
        {{ tick.label }}
      </span>
    </div>
    <!-- Spectrum canvas -->
    <canvas ref="spectrumCanvas" class="spectrum" :width="canvasWidth" :height="spectrumHeight"></canvas>
    <!-- Waterfall canvas -->
    <canvas ref="waterfallCanvas" class="waterfall" :width="canvasWidth" :height="waterfallHeight"></canvas>
    <!-- Filter overlay (dimmed outside, highlighted inside, draggable edges) -->
    <div class="filter-overlay" :style="filterStyle">
      <div class="filter-dim left"></div>
      <div class="filter-dim right"></div>
      <div class="filter-passband" @mousedown="onCenterDown">
        <div class="filter-handle left" @mousedown.stop="onLeftDown"></div>
        <div class="filter-handle right" @mousedown.stop="onRightDown"></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useWaterfall } from '../composables/useWaterfall'
import { debounce } from '../composables/useDebounce'

const props = defineProps<{
  centerFreq: number
  sampleRate: number
  filterLow?: number
  filterHigh?: number
}>()

const emit = defineEmits<{
  'update:filter': [low: number, high: number]
}>()

const containerRef = ref<HTMLDivElement | null>(null)
const spectrumCanvas = ref<HTMLCanvasElement | null>(null)
const waterfallCanvas = ref<HTMLCanvasElement | null>(null)
const canvasWidth = ref(800)
const spectrumHeight = ref(200)
const waterfallHeight = ref(300)

const { setCanvases, fftSize } = useWaterfall()

// Debounced filter emission so dragging an edge doesn't flood the backend
// with one POST /api/filter per mousemove event.
const emitFilter = debounce((low: number, high: number) => {
  emit('update:filter', low, high)
}, 40)

// --- Filter drag logic ---
type DragMode = 'none' | 'left' | 'right' | 'center'
let dragMode: DragMode = 'none'
let dragStartX = 0
let dragStartLow = 0
let dragStartHigh = 0

function pxToFreq(px: number): number {
  const w = canvasWidth.value
  if (w <= 0) return 0
  return (px / w) * props.sampleRate - props.sampleRate / 2
}

function freqToPercent(freq: number): number {
  const halfBW = props.sampleRate / 2
  return ((freq + halfBW) / props.sampleRate) * 100
}

function onLeftDown(e: MouseEvent) {
  e.preventDefault()
  dragMode = 'left'
  dragStartX = e.clientX
  dragStartLow = props.filterLow || 0
  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('mouseup', onMouseUp)
}

function onRightDown(e: MouseEvent) {
  e.preventDefault()
  dragMode = 'right'
  dragStartX = e.clientX
  dragStartHigh = props.filterHigh || 0
  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('mouseup', onMouseUp)
}

function onCenterDown(e: MouseEvent) {
  e.preventDefault()
  dragMode = 'center'
  dragStartX = e.clientX
  dragStartLow = props.filterLow || 0
  dragStartHigh = props.filterHigh || 0
  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('mouseup', onMouseUp)
}

function onMouseMove(e: MouseEvent) {
  const dx = e.clientX - dragStartX
  const df = (dx / canvasWidth.value) * props.sampleRate

  if (dragMode === 'left') {
    const newLow = Math.min(dragStartLow + df, (props.filterHigh || 0) - 100)
    emitFilter(Math.round(newLow), props.filterHigh || 0)
  } else if (dragMode === 'right') {
    const newHigh = Math.max(dragStartHigh + df, (props.filterLow || 0) + 100)
    emitFilter(props.filterLow || 0, Math.round(newHigh))
  } else if (dragMode === 'center') {
    const newLow = dragStartLow + df
    const newHigh = dragStartHigh + df
    const halfBW = props.sampleRate / 2
    // Clamp to passband
    if (newLow > -halfBW && newHigh < halfBW) {
      emitFilter(Math.round(newLow), Math.round(newHigh))
    }
  }
}

function onMouseUp() {
  dragMode = 'none'
  window.removeEventListener('mousemove', onMouseMove)
  window.removeEventListener('mouseup', onMouseUp)
}

onUnmounted(() => {
  window.removeEventListener('mousemove', onMouseMove)
  window.removeEventListener('mouseup', onMouseUp)
  window.removeEventListener('resize', updateCanvasSize)
})

const freqTicks = computed(() => {
  const ticks: { freq: number; pos: number; label: string }[] = []
  const halfBW = props.sampleRate / 2
  const numTicks = 11
  for (let i = 0; i < numTicks; i++) {
    const frac = i / (numTicks - 1)
    const freq = props.centerFreq + (frac - 0.5) * props.sampleRate
    const pos = frac * 100
    let label: string
    if (freq >= 1e6) {
      label = (freq / 1e6).toFixed(1) + 'M'
    } else if (freq >= 1e3) {
      label = (freq / 1e3).toFixed(0) + 'k'
    } else {
      label = freq.toFixed(0)
    }
    ticks.push({ freq, pos, label })
  }
  return ticks
})

const filterStyle = computed(() => {
  const low = props.filterLow ?? 0
  const high = props.filterHigh ?? 0
  const halfBW = props.sampleRate / 2
  const leftPct = freqToPercent(low)
  const rightPct = freqToPercent(high)
  return {
    '--left-pct': leftPct + '%',
    '--right-pct': (100 - rightPct) + '%',
    display: 'block',
  }
})

onMounted(() => {
  if (spectrumCanvas.value && waterfallCanvas.value) {
    setCanvases(spectrumCanvas.value, waterfallCanvas.value)
  }
  // Responsive canvas
  updateCanvasSize()
  window.addEventListener('resize', updateCanvasSize)
})

function updateCanvasSize() {
  const container = spectrumCanvas.value?.parentElement
  if (container) {
    canvasWidth.value = container.clientWidth
    // Auto-fit height: split available space between spectrum (35%) and waterfall (65%)
    const containerHeight = container.clientHeight
    const axisHeight = 21 // freq-axis height + border
    const availableHeight = containerHeight - axisHeight
    if (availableHeight > 0) {
      spectrumHeight.value = Math.floor(availableHeight * 0.35)
      waterfallHeight.value = Math.floor(availableHeight * 0.65)
    }
  }
}
</script>

<style scoped>
.waterfall-container {
  position: relative;
  width: 100%;
  height: 100%;
  background: #0a0a0a;
  border: 1px solid #222;
  overflow: hidden;
}

.freq-axis {
  position: relative;
  height: 20px;
  background: #111;
  font-size: 10px;
  color: #888;
  border-bottom: 1px solid #333;
  overflow: hidden;
}

.freq-axis span {
  position: absolute;
  top: 4px;
  transform: translateX(-50%);
  white-space: nowrap;
}

.freq-axis span.tick-left {
  transform: translateX(5%);
}

.freq-axis span.tick-right {
  transform: translateX(-105%);
}

.spectrum {
  display: block;
  width: 100%;
}

.waterfall {
  display: block;
  width: 100%;
}

.filter-overlay {
  position: absolute;
  top: 20px;
  bottom: 0;
  left: 0;
  right: 0;
  pointer-events: none;
  z-index: 10;
}

/* Dimmed regions outside the filter passband */
.filter-dim {
  position: absolute;
  top: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
}

.filter-dim.left {
  left: 0;
  width: var(--left-pct, 50%);
  border-right: 1px solid rgba(100, 180, 255, 0.7);
}

.filter-dim.right {
  right: 0;
  width: var(--right-pct, 50%);
  border-left: 1px solid rgba(100, 180, 255, 0.7);
}

/* Passband region (draggable center) */
.filter-passband {
  position: absolute;
  top: 0;
  bottom: 0;
  left: var(--left-pct, 50%);
  right: var(--right-pct, 50%);
  background: rgba(100, 180, 255, 0.06);
  border-left: 1px solid rgba(100, 180, 255, 0.7);
  border-right: 1px solid rgba(100, 180, 255, 0.7);
  cursor: grab;
  pointer-events: auto;
}

.filter-passband:active {
  cursor: grabbing;
}

/* Drag handles on left and right edges */
.filter-handle {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 8px;
  pointer-events: auto;
  cursor: ew-resize;
}

.filter-handle.left {
  left: -4px;
}

.filter-handle.right {
  right: -4px;
}

.filter-handle:hover {
  background: rgba(100, 180, 255, 0.2);
}
</style>
