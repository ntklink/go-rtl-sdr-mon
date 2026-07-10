<template>
  <div class="panel noaa-panel">
    <h3>{{ t('noaa.title') }}</h3>

    <!-- Satellite selection -->
    <div class="control-group">
      <label>{{ t('noaa.satellite') }}</label>
      <div class="sat-buttons">
        <button v-for="sat in satellites" :key="sat.name" class="sat-btn" :class="{ active: isActive(sat) }"
          @click="selectSatellite(sat)">
          {{ sat.name }}
          <span class="sat-freq">{{ (sat.frequency / 1e6).toFixed(4) }} MHz</span>
        </button>
      </div>
    </div>

    <!-- Statistics -->
    <div class="stats-bar" v-if="stats">
      <span class="stat-item">
        <span class="stat-label">{{ t('noaa.lines') }}:</span>
        <span class="stat-value" :class="{ zero: stats.lines === 0 }">{{ stats.lines }}</span>
      </span>
      <span class="stat-item">
        <span class="stat-label">{{ t('noaa.sync') }}:</span>
        <span class="stat-value" :class="{ zero: stats.sync === 0 }">{{ stats.sync }}</span>
      </span>
      <span class="stat-item signal-item">
        <span class="stat-label">{{ t('noaa.signal') }}</span>
        <span class="signal-bar-wrap">
          <span class="signal-bar-fill" :class="signalClass" :style="{ width: signalPct + '%' }"></span>
        </span>
        <span class="signal-text" :class="signalClass">{{ signalLabel }}</span>
      </span>
    </div>

    <div class="tip-bar" v-if="stats && stats.signalLevel < 0.01">
      {{ t('noaa.noSignal') }}
    </div>
    <div class="tip-bar" v-else-if="stats && stats.sync === 0 && stats.signalLevel >= 0.01">
      {{ t('noaa.noSync') }}
    </div>

    <!-- Image display -->
    <div class="image-container">
      <div v-if="lines.length === 0" class="no-data">
        {{ t('noaa.noImage') }}
      </div>
      <canvas v-else ref="canvasRef" :width="APT_LINE_WIDTH" class="apt-canvas"></canvas>
    </div>

    <!-- Controls -->
    <div class="control-row">
      <button class="btn-reset" @click="resetImage">{{ t('noaa.reset') }}</button>
      <button class="btn-save" @click="saveImage" :disabled="lines.length === 0">{{ t('noaa.save') }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, nextTick, computed } from 'vue'
import { useNoAA, type Satellite } from '../composables/useNoAA'
import { useApi } from '../composables/useApi'
import { useI18n } from '../composables/useI18n'
import { useStatus } from '../composables/useStatus'

const { lines, receivedLines, stats, resetImage, saveImage } = useNoAA()
const api = useApi()
const { t } = useI18n()
const { status } = useStatus()

const APT_LINE_WIDTH = 2080
const satellites = ref<Satellite[]>([])
const canvasRef = ref<HTMLCanvasElement | null>(null)

// Signal level display helpers
const signalPct = computed(() => Math.min(100, Math.max(0, (stats.value.signalLevel || 0) * 500)))
const signalClass = computed(() => {
  const s = stats.value.signalLevel || 0
  if (s < 0.01) return 'sig-none'
  if (s < 0.05) return 'sig-weak'
  return 'sig-good'
})
const signalLabel = computed(() => {
  const s = stats.value.signalLevel || 0
  if (s < 0.01) return t('noaa.signalNone')
  if (s < 0.05) return t('noaa.signalWeak')
  return t('noaa.signalGood')
})

// Number of rows already rendered to the canvas. Used to draw only the newly
// received lines incrementally instead of redrawing the whole image every
// update (the old code did a full O(width*height) redraw both on a 200ms
// timer and on every new line).
let drawnLines = 0

async function loadSatellites() {
  try {
    const resp = await api.getNOAASatellites()
    satellites.value = resp.satellites || []
  } catch {
    // ignore
  }
}

function isActive(sat: Satellite): boolean {
  return Math.abs(status.value.CenterFreq - sat.frequency) < 5000
}

async function selectSatellite(sat: Satellite) {
  try {
    await api.setFrequency(sat.frequency)
  } catch (e) {
    console.error('Set frequency failed:', e)
  }
}

// Paint rows [yStart, yEnd) from lines.value into the canvas.
function drawRows(ctx: CanvasRenderingContext2D, yStart: number, yEnd: number) {
  const w = APT_LINE_WIDTH
  const img = ctx.createImageData(w, yEnd - yStart)
  for (let y = yStart; y < yEnd; y++) {
    const line = lines.value[y]
    if (!line) continue
    const len = Math.min(w, line.length)
    for (let x = 0; x < len; x++) {
      const v = line[x]
      const idx = ((y - yStart) * w + x) * 4
      img.data[idx] = v
      img.data[idx + 1] = v
      img.data[idx + 2] = v
      img.data[idx + 3] = 255
    }
  }
  ctx.putImageData(img, 0, yStart)
}

function drawCanvas() {
  const canvas = canvasRef.value
  if (!canvas) return
  const n = lines.value.length
  if (n === 0) {
    drawnLines = 0
    return
  }
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  if (canvas.width !== APT_LINE_WIDTH) {
    canvas.width = APT_LINE_WIDTH
    drawnLines = 0
  }

  // Keep the backing height equal to n (no blank space below the image),
  // preserving already-drawn rows when growing.
  if (canvas.height !== n) {
    if (n > drawnLines && drawnLines > 0 && canvas.height > 0) {
      // Growth: snapshot the valid rows, resize (which clears), restore.
      const keep = Math.min(drawnLines, canvas.height)
      const tmp = document.createElement('canvas')
      tmp.width = canvas.width
      tmp.height = keep
      tmp.getContext('2d')!.drawImage(canvas, 0, 0, tmp.width, keep, 0, 0, tmp.width, keep)
      canvas.height = n
      ctx.drawImage(tmp, 0, 0)
      // drawnLines unchanged (content preserved)
    } else {
      // Shrink (reset) or fresh canvas: clear and redraw from scratch.
      canvas.height = n
      drawnLines = 0
    }
  }

  if (n > drawnLines) {
    // New rows appended: paint only the delta.
    drawRows(ctx, drawnLines, n)
    drawnLines = n
  } else if (n === drawnLines && n > 0) {
    // Buffer is full and shifted by one (oldest dropped): scroll up by 1
    // and paint the new bottom row.
    ctx.drawImage(canvas, 0, -1)
    drawRows(ctx, n - 1, n)
  }
}

onMounted(() => {
  loadSatellites()
  nextTick(drawCanvas)
})

// Redraw incrementally whenever a new line arrives (receivedLines increments
// even when the buffer is full and shifts). Replaces the old 200ms full-redraw
// interval.
const stopWatch = watch(receivedLines, () => {
  nextTick(drawCanvas)
})

onUnmounted(() => {
  stopWatch()
})
</script>

<style scoped>
.noaa-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.stats-bar {
  display: flex;
  gap: 12px;
  padding: 4px 0;
}

.stat-item {
  display: flex;
  align-items: center;
  gap: 4px;
}

.stat-label {
  font-size: 11px;
  opacity: 0.7;
}

.stat-value {
  font-weight: bold;
  color: var(--accent-color, #4fc3f7);
}

.stat-value.zero {
  color: var(--text-muted, #666);
}

.signal-item {
  flex: 1;
  min-width: 0;
}

.signal-bar-wrap {
  flex: 1;
  height: 10px;
  background: #1a1a2e;
  border-radius: 3px;
  overflow: hidden;
  min-width: 40px;
}

.signal-bar-fill {
  display: block;
  height: 100%;
  transition: width 0.5s ease;
  border-radius: 3px;
}

.signal-bar-fill.sig-none {
  background: #555;
}

.signal-bar-fill.sig-weak {
  background: linear-gradient(90deg, #ff9800, #ffc107);
}

.signal-bar-fill.sig-good {
  background: linear-gradient(90deg, #4caf50, #8bc34a);
}

.signal-text {
  font-size: 10px;
  font-weight: bold;
  min-width: 20px;
}

.signal-text.sig-none {
  color: #666;
}

.signal-text.sig-weak {
  color: #ffc107;
}

.signal-text.sig-good {
  color: #8bc34a;
}

.tip-bar {
  font-size: 11px;
  padding: 6px 8px;
  background: rgba(255, 193, 7, 0.1);
  border: 1px solid rgba(255, 193, 7, 0.3);
  border-radius: 4px;
  opacity: 0.85;
  line-height: 1.4;
}

.sat-buttons {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.sat-btn {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 6px 10px;
  border: 1px solid var(--border-color, #333);
  background: transparent;
  color: inherit;
  border-radius: 4px;
  cursor: pointer;
  font-size: 12px;
  transition: background 0.15s;
}

.sat-btn:hover {
  background: rgba(255, 255, 255, 0.05);
}

.sat-btn.active {
  border-color: var(--accent-color, #4fc3f7);
  background: rgba(79, 195, 247, 0.1);
}

.sat-freq {
  font-size: 11px;
  opacity: 0.7;
}

.image-container {
  flex: 1;
  min-height: 100px;
  overflow-y: auto;
  border: 1px solid var(--border-color, #333);
  border-radius: 4px;
  background: #000;
}

.no-data {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100px;
  opacity: 0.5;
  font-size: 12px;
}

.apt-canvas {
  display: block;
  width: 100%;
  image-rendering: pixelated;
  image-rendering: crisp-edges;
}

.control-row {
  display: flex;
  gap: 8px;
}

.btn-reset,
.btn-save {
  flex: 1;
  padding: 6px 12px;
  border: 1px solid var(--border-color, #333);
  background: transparent;
  color: inherit;
  border-radius: 4px;
  cursor: pointer;
  font-size: 12px;
  transition: background 0.15s;
}

.btn-reset:hover,
.btn-save:hover {
  background: rgba(255, 255, 255, 0.05);
}

.btn-save:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
</style>
