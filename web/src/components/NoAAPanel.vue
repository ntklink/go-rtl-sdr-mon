<template>
  <div class="panel noaa-panel">
    <h3>{{ t('noaa.title') }}</h3>

    <!-- Satellite selection -->
    <div class="control-group">
      <label>{{ t('noaa.satellite') }}</label>
      <div class="sat-buttons">
        <button
          v-for="sat in satellites"
          :key="sat.name"
          class="sat-btn"
          :class="{ active: isActive(sat) }"
          @click="selectSatellite(sat)"
        >
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
    </div>

    <div class="tip-bar" v-if="stats && stats.sync === 0">
      {{ t('noaa.tip') }}
    </div>

    <!-- Image display -->
    <div class="image-container">
      <div v-if="lines.length === 0" class="no-data">
        {{ t('noaa.noImage') }}
      </div>
      <canvas
        v-else
        ref="canvasRef"
        :width="APT_LINE_WIDTH"
        :height="lines.length"
        class="apt-canvas"
      ></canvas>
    </div>

    <!-- Controls -->
    <div class="control-row">
      <button class="btn-reset" @click="resetImage">{{ t('noaa.reset') }}</button>
      <button class="btn-save" @click="saveImage" :disabled="lines.length === 0">{{ t('noaa.save') }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useNoAA, type Satellite } from '../composables/useNoAA'
import { useApi } from '../composables/useApi'
import { useI18n } from '../composables/useI18n'
import { useStatus } from '../composables/useStatus'

const { lines, stats, resetImage, saveImage } = useNoAA()
const api = useApi()
const { t } = useI18n()
const { status } = useStatus()

const APT_LINE_WIDTH = 2080
const satellites = ref<Satellite[]>([])
const canvasRef = ref<HTMLCanvasElement | null>(null)
let drawTimer: ReturnType<typeof setInterval> | null = null

async function loadSatellites() {
  try {
    const resp = await fetch('/api/noaa/satellites')
    if (resp.ok) {
      const data = await resp.json()
      satellites.value = data.satellites || []
    }
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

function drawCanvas() {
  const canvas = canvasRef.value
  if (!canvas || lines.value.length === 0) return

  // Adjust canvas height
  if (canvas.height !== lines.value.length) {
    canvas.height = lines.value.length
  }

  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const imageData = ctx.createImageData(canvas.width, canvas.height)
  for (let y = 0; y < lines.value.length; y++) {
    const line = lines.value[y]
    for (let x = 0; x < canvas.width && x < line.length; x++) {
      const v = line[x]
      const idx = (y * canvas.width + x) * 4
      imageData.data[idx] = v
      imageData.data[idx + 1] = v
      imageData.data[idx + 2] = v
      imageData.data[idx + 3] = 255
    }
  }
  ctx.putImageData(imageData, 0, 0)
}

onMounted(() => {
  loadSatellites()
  // Redraw canvas at ~5 fps
  drawTimer = setInterval(drawCanvas, 200)
})

onUnmounted(() => {
  if (drawTimer) {
    clearInterval(drawTimer)
    drawTimer = null
  }
})

// Redraw when lines change
watch(() => lines.value.length, () => {
  nextTick(drawCanvas)
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
