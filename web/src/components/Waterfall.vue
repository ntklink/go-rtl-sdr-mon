<template>
  <div class="waterfall-container">
    <!-- Frequency axis -->
    <div class="freq-axis" ref="freqAxis">
      <span
        v-for="(tick, i) in freqTicks"
        :key="tick.freq"
        :class="{ 'tick-left': i === 0, 'tick-right': i === freqTicks.length - 1 }"
        :style="{ left: tick.pos + '%' }"
      >
        {{ tick.label }}
      </span>
    </div>
    <!-- Spectrum canvas -->
    <canvas ref="spectrumCanvas" class="spectrum" :width="canvasWidth" :height="spectrumHeight"></canvas>
    <!-- Waterfall canvas -->
    <canvas ref="waterfallCanvas" class="waterfall" :width="canvasWidth" :height="waterfallHeight"></canvas>
    <!-- Filter overlay -->
    <div class="filter-overlay" :style="filterStyle">
      <div class="filter-line left"></div>
      <div class="filter-line right"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useWaterfall } from '../composables/useWaterfall'

const props = defineProps<{
  centerFreq: number
  sampleRate: number
  filterLow?: number
  filterHigh?: number
}>()

const spectrumCanvas = ref<HTMLCanvasElement | null>(null)
const waterfallCanvas = ref<HTMLCanvasElement | null>(null)
const canvasWidth = ref(800)
const spectrumHeight = ref(200)
const waterfallHeight = ref(300)

const { setCanvases, fftSize } = useWaterfall()

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
  if (!props.filterLow || !props.filterHigh) return { display: 'none' }
  const halfBW = props.sampleRate / 2
  const leftFrac = (props.filterLow + halfBW) / props.sampleRate
  const rightFrac = (props.filterHigh + halfBW) / props.sampleRate
  return {
    left: (leftFrac * 100) + '%',
    width: ((rightFrac - leftFrac) * 100) + '%',
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

watch([canvasWidth, spectrumHeight, waterfallHeight], () => {
  updateCanvasSize()
})
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
  background: rgba(255, 255, 0, 0.05);
  border-left: 1px solid rgba(255, 255, 0, 0.5);
  border-right: 1px solid rgba(255, 255, 0, 0.5);
  pointer-events: none;
  z-index: 10;
}

.filter-line {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 1px;
  background: rgba(255, 255, 0, 0.6);
}

.filter-line.left { left: 0; }
.filter-line.right { right: 0; }
</style>
