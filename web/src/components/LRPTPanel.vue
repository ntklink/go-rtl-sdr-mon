<template>
  <div class="panel lrpt-panel">
    <h3>{{ t('lrpt.title') }}</h3>

    <!-- Satellite selection -->
    <div class="control-group">
      <label>{{ t('lrpt.satellite') }}</label>
      <div class="sat-buttons">
        <button v-for="sat in satellites" :key="sat.name" class="sat-btn" :class="{ active: isActive(sat) }"
          @click="selectSatellite(sat)">
          {{ sat.name }}
          <span class="sat-freq">{{ (sat.frequency / 1e6).toFixed(4) }} MHz</span>
        </button>
      </div>
    </div>

    <!-- Signal status -->
    <div class="stats-bar">
      <span class="lock-badge" :class="stats.locked ? 'locked' : 'unlocked'">
        {{ stats.locked ? t('lrpt.locked') : t('lrpt.unlocked') }}
      </span>
      <span class="stat-item signal-item">
        <span class="stat-label">{{ t('lrpt.signal') }}</span>
        <span class="signal-bar-wrap">
          <span class="signal-bar-fill" :class="qualityClass" :style="{ width: stats.signalQ + '%' }"></span>
        </span>
        <span class="signal-text" :class="qualityClass">{{ Math.round(stats.signalQ) }}%</span>
      </span>
      <span class="stat-item" v-if="stats.locked">
        <span class="stat-label">{{ t('lrpt.freqOffset') }}:</span>
        <span class="stat-value">{{ Math.round(stats.freqOffset) }} Hz</span>
      </span>
    </div>

    <!-- Counters + constellation -->
    <div class="mid-row">
      <div class="counters">
        <span class="counter-label">{{ t('lrpt.frames') }}</span>
        <span class="counter-value">
          <span class="stat-value" :class="{ zero: stats.framesOK === 0 }">{{ stats.framesOK }}</span>
          <span class="stat-sub" v-if="stats.framesBad > 0">/ {{ stats.framesBad }} bad</span>
        </span>
        <span class="counter-label">{{ t('lrpt.packets') }}</span>
        <span class="counter-value">
          <span class="stat-value" :class="{ zero: stats.packets === 0 }">{{ stats.packets }}</span>
        </span>
        <span class="counter-label">{{ t('lrpt.rs') }}</span>
        <span class="counter-value">
          <span class="stat-value">{{ stats.rsCorrected }}</span>
        </span>
      </div>
      <canvas ref="constCanvas" width="96" height="96" class="const-canvas" :title="t('lrpt.constellation')"></canvas>
    </div>

    <div class="tip-bar" v-if="!stats.locked">
      {{ t('lrpt.noSignal') }}
    </div>
    <div class="tip-bar" v-else-if="stats.locked && stats.framesOK === 0">
      {{ t('lrpt.noFrames') }}
    </div>

    <!-- Channel selector -->
    <div class="control-group" v-if="apids.length > 0">
      <label>{{ t('lrpt.channel') }}</label>
      <div class="chan-buttons">
        <button v-for="apid in apids" :key="apid" class="chan-btn" :class="{ active: apid === selectedApid }"
          @click="selectChannel(apid)">
          APID {{ apid }}
        </button>
      </div>
    </div>

    <!-- Image display -->
    <div class="image-container">
      <div v-if="selectedApid === null" class="no-data">
        {{ t('lrpt.noImage') }}
      </div>
      <canvas v-else ref="imgCanvas" :width="LRPT_IMAGE_WIDTH" class="lrpt-canvas"></canvas>
    </div>

    <!-- Controls -->
    <div class="control-row">
      <button class="btn-reset" @click="resetImage">{{ t('lrpt.reset') }}</button>
      <button class="btn-save" @click="selectedApid !== null && saveImage(selectedApid)"
        :disabled="selectedApid === null">{{ t('lrpt.save') }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, nextTick, computed } from 'vue'
import { useLRPT, LRPT_IMAGE_WIDTH, type Satellite } from '../composables/useLRPT'
import { useApi } from '../composables/useApi'
import { useI18n } from '../composables/useI18n'
import { useStatus } from '../composables/useStatus'

const {
  apids, stats, constellation, segCounter,
  getChannelCanvas, getChannelHeight, lastSegment, resetImage, saveImage,
} = useLRPT()
const api = useApi()
const { t } = useI18n()
const { status } = useStatus()

const satellites = ref<Satellite[]>([])
const selectedApid = ref<number | null>(null)
const imgCanvas = ref<HTMLCanvasElement | null>(null)
const constCanvas = ref<HTMLCanvasElement | null>(null)

const qualityClass = computed(() => {
  const q = stats.value.signalQ
  if (q < 20) return 'sig-none'
  if (q < 55) return 'sig-weak'
  return 'sig-good'
})

async function loadSatellites() {
  try {
    const resp = await api.getLRPTSatellites()
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

function selectChannel(apid: number) {
  selectedApid.value = apid
  nextTick(() => redrawFull())
}

// Full blit of the selected channel's offscreen canvas.
function redrawFull() {
  const apid = selectedApid.value
  const canvas = imgCanvas.value
  if (apid === null || !canvas) return
  const src = getChannelCanvas(apid)
  const h = getChannelHeight(apid)
  if (!src || h === 0) return
  if (canvas.height !== h) canvas.height = h
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  ctx.drawImage(src, 0, 0, LRPT_IMAGE_WIDTH, h, 0, 0, LRPT_IMAGE_WIDTH, h)
}

// Incremental update on each received segment.
const stopSegWatch = watch(segCounter, () => {
  // Auto-select the first channel once data appears
  if (selectedApid.value === null && apids.value.length > 0) {
    selectedApid.value = apids.value[0]
    nextTick(() => redrawFull())
    return
  }
  const seg = lastSegment()
  const apid = selectedApid.value
  const canvas = imgCanvas.value
  if (!seg || apid === null || !canvas) return
  if (seg.apid !== apid) return
  const h = getChannelHeight(apid)
  if (canvas.height !== h) {
    canvas.height = h
    redrawFull()
    return
  }
  const src = getChannelCanvas(apid)
  const ctx = canvas.getContext('2d')
  if (!src || !ctx) return
  ctx.drawImage(src, seg.x, seg.y, seg.w, seg.h, seg.x, seg.y, seg.w, seg.h)
})

// Keep the selection valid when channels appear or are cleared.
const stopApidWatch = watch(apids, (list) => {
  if (selectedApid.value !== null && !list.includes(selectedApid.value)) {
    selectedApid.value = list.length > 0 ? list[0] : null
    nextTick(() => redrawFull())
  }
})

// Constellation diagram redraw on stats updates.
const stopConstWatch = watch(constellation, (pts) => {
  const canvas = constCanvas.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  const w = canvas.width
  ctx.fillStyle = '#0a0a14'
  ctx.fillRect(0, 0, w, w)
  ctx.strokeStyle = '#333'
  ctx.beginPath()
  ctx.moveTo(w / 2, 0)
  ctx.lineTo(w / 2, w)
  ctx.moveTo(0, w / 2)
  ctx.lineTo(w, w / 2)
  ctx.stroke()
  ctx.fillStyle = stats.value.locked ? '#8bc34a' : '#4fc3f7'
  const half = w / 2
  for (let i = 0; i + 1 < pts.length; i += 2) {
    const x = half + (pts[i] / 127) * (half - 4)
    const y = half - (pts[i + 1] / 127) * (half - 4)
    ctx.fillRect(x - 1, y - 1, 2, 2)
  }
})

onMounted(() => {
  loadSatellites()
  nextTick(() => redrawFull())
})

onUnmounted(() => {
  stopSegWatch()
  stopConstWatch()
  stopApidWatch()
})
</script>

<style scoped>
.lrpt-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.stats-bar {
  display: flex;
  gap: 12px;
  padding: 4px 0;
  align-items: center;
}

.lock-badge {
  font-size: 10px;
  font-weight: bold;
  padding: 2px 8px;
  border-radius: 3px;
}

.lock-badge.locked {
  background: rgba(139, 195, 74, 0.2);
  color: #8bc34a;
  border: 1px solid rgba(139, 195, 74, 0.5);
}

.lock-badge.unlocked {
  background: rgba(120, 120, 120, 0.15);
  color: #888;
  border: 1px solid #444;
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

.stat-sub {
  font-size: 10px;
  opacity: 0.6;
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
  min-width: 28px;
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

.mid-row {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}

.counters {
  flex: 1;
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 4px 10px;
  align-content: start;
  font-size: 12px;
}

.counter-label {
  font-size: 11px;
  opacity: 0.7;
  text-align: right;
  align-self: baseline;
}

.counter-value {
  display: flex;
  gap: 4px;
  align-items: baseline;
  font-variant-numeric: tabular-nums;
}

.const-canvas {
  border: 1px solid var(--border-color, #333);
  border-radius: 4px;
  background: #0a0a14;
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

.chan-buttons {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.chan-btn {
  padding: 4px 10px;
  border: 1px solid var(--border-color, #333);
  background: transparent;
  color: inherit;
  border-radius: 4px;
  cursor: pointer;
  font-size: 11px;
  transition: background 0.15s;
}

.chan-btn:hover {
  background: rgba(255, 255, 255, 0.05);
}

.chan-btn.active {
  border-color: var(--accent-color, #4fc3f7);
  background: rgba(79, 195, 247, 0.1);
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

.lrpt-canvas {
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
