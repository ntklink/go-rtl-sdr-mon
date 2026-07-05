<template>
  <div class="panel receiver-panel">
    <h3>接收机选项</h3>

    <div class="control-group">
      <label>解调模式</label>
      <SelectRoot v-model="selectedDemod">
        <SelectTrigger class="reka-select-trigger">
          <SelectValue placeholder="选择..." />
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M6 9l6 6 6-6" />
          </svg>
        </SelectTrigger>
        <SelectPortal>
          <SelectContent class="reka-select-content" position="popper" :side-offset="4">
            <SelectItem v-for="d in demods" :key="d" :value="d" class="reka-select-item">
              {{ d }}
            </SelectItem>
          </SelectContent>
        </SelectPortal>
      </SelectRoot>
    </div>

    <div class="control-group">
      <label>滤波器低截止 (Hz)</label>
      <input
        type="number"
        :value="filterLow"
        @change="onFilterLowChange"
        class="input"
        step="100"
      />
    </div>

    <div class="control-group">
      <label>滤波器高截止 (Hz)</label>
      <input
        type="number"
        :value="filterHigh"
        @change="onFilterHighChange"
        class="input"
        step="100"
      />
    </div>

    <div class="control-group">
      <label>滤波器预设</label>
      <div class="preset-buttons">
        <button @click="setFilterPreset(-5000, 5000)">±5k</button>
        <button @click="setFilterPreset(-10000, 10000)">±10k</button>
        <button @click="setFilterPreset(-15000, 15000)">±15k</button>
        <button @click="setFilterPreset(-75000, 75000)">WFM ±75k</button>
        <button @click="setFilterPreset(-3000, 3000)">SSB ±3k</button>
        <button @click="setFilterPreset(300, 3000)">USB</button>
        <button @click="setFilterPreset(-3000, -300)">LSB</button>
      </div>
    </div>

    <div class="control-group">
      <label>静噪电平 (dBFS)</label>
      <div class="slider-row">
        <SliderRoot
          v-model="squelchSlider"
          :min="-150"
          :max="0"
          :step="1"
          class="reka-slider-root"
          @update:model-value="onSquelchChange"
        >
          <SliderTrack class="reka-slider-track">
            <SliderRange class="reka-slider-range" />
          </SliderTrack>
          <SliderThumb class="reka-slider-thumb" />
        </SliderRoot>
        <span class="value-display">{{ squelchLevel === -150 ? '关闭' : squelchLevel.toFixed(0) + ' dB' }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { SelectRoot, SelectTrigger, SelectValue, SelectContent, SelectItem, SelectPortal } from 'reka-ui'
import { SliderRoot, SliderTrack, SliderRange, SliderThumb } from 'reka-ui'
import { useApi } from '../composables/useApi'

const api = useApi()

const props = defineProps<{
  demod: string
  filterLow: number
  filterHigh: number
}>()

const emit = defineEmits<{
  'update:demod': [value: string]
  'update:filter': [low: number, high: number]
}>()

const demods = ['NFM', 'WFM', 'WFM-Stereo', 'AM', 'AM-Sync', 'SSB', 'OFF']
const squelchLevel = ref(-150)

const selectedDemod = computed({
  get: () => props.demod,
  set: (val: string) => emit('update:demod', val),
})

const squelchSlider = computed({
  get: () => [squelchLevel.value],
  set: (val: number[]) => { squelchLevel.value = val[0] },
})

async function onFilterLowChange(e: Event) {
  const val = parseInt((e.target as HTMLInputElement).value)
  emit('update:filter', val, props.filterHigh)
}

async function onFilterHighChange(e: Event) {
  const val = parseInt((e.target as HTMLInputElement).value)
  emit('update:filter', props.filterLow, val)
}

function setFilterPreset(low: number, high: number) {
  emit('update:filter', low, high)
}

async function onSquelchChange() {
  try {
    await api.setSquelch(squelchLevel.value)
  } catch (e) {
    console.error('Set squelch failed:', e)
  }
}
</script>

<style scoped>
.receiver-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.slider-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.preset-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.preset-buttons button {
  padding: 3px 8px;
  background: #222;
  border: 1px solid #333;
  color: #ccc;
  border-radius: 3px;
  cursor: pointer;
  font-size: 11px;
}

.preset-buttons button:hover {
  background: #333;
}
</style>
