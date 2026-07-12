<template>
  <div class="panel receiver-panel">
    <h3>{{ t('rx.title') }}</h3>

    <div class="control-group">
      <label>{{ t('rx.demod') }}</label>
      <SelectRoot v-model="selectedDemod">
        <SelectTrigger class="reka-select-trigger">
          <span class="select-display">{{ selectedDemod || '...' }}</span>
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
      <label>{{ t('rx.filterLow') }}</label>
      <input type="number" :value="filterLow" @change="onFilterLowChange" class="input" step="100" />
    </div>

    <div class="control-group">
      <label>{{ t('rx.filterHigh') }}</label>
      <input type="number" :value="filterHigh" @change="onFilterHighChange" class="input" step="100" />
    </div>

    <div class="control-group">
      <label>{{ t('rx.filterPreset') }}</label>
      <div class="preset-buttons">
        <button @click="onFilterPreset('wide')">Wide</button>
        <button @click="onFilterPreset('normal')">Normal</button>
        <button @click="onFilterPreset('narrow')">Narrow</button>
      </div>
    </div>

    <div class="control-group">
      <label>{{ t('rx.filterShape') }}</label>
      <SelectRoot v-model="filterShape" @update:model-value="onFilterShapeChange">
        <SelectTrigger class="reka-select-trigger">
          <span class="select-display">{{ filterShape || '...' }}</span>
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M6 9l6 6 6-6" />
          </svg>
        </SelectTrigger>
        <SelectPortal>
          <SelectContent class="reka-select-content" position="popper" :side-offset="4">
            <SelectItem value="soft" class="reka-select-item">Soft</SelectItem>
            <SelectItem value="normal" class="reka-select-item">Normal</SelectItem>
            <SelectItem value="sharp" class="reka-select-item">Sharp</SelectItem>
          </SelectContent>
        </SelectPortal>
      </SelectRoot>
    </div>

    <div class="control-group">
      <label>{{ t('rx.squelch') }}</label>
      <div class="slider-row">
        <SliderRoot v-model="squelchSlider" :min="-150" :max="0" :step="1" class="reka-slider-root"
          @update:model-value="onSquelchChange">
          <SliderTrack class="reka-slider-track">
            <SliderRange class="reka-slider-range" />
          </SliderTrack>
          <SliderThumb class="reka-slider-thumb" />
        </SliderRoot>
        <span class="value-display">{{ squelchLevel === -150 ? t('rx.squelchOff') : squelchLevel.toFixed(0) + ' dB'
          }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { SelectRoot, SelectTrigger, SelectContent, SelectItem, SelectPortal } from 'reka-ui'
import { SliderRoot, SliderTrack, SliderRange, SliderThumb } from 'reka-ui'
import { useApi } from '../composables/useApi'
import { useI18n } from '../composables/useI18n'
import { useStatus } from '../composables/useStatus'
import { debounce } from '../composables/useDebounce'

const api = useApi()
const { t } = useI18n()
const { status, statusLoaded } = useStatus()

const props = defineProps<{
  demod: string
  filterLow: number
  filterHigh: number
}>()

const emit = defineEmits<{
  'update:demod': [value: string]
  'update:filter': [low: number, high: number]
}>()

const demods = ['OFF', 'Raw I/Q', 'AM', 'AM-Sync', 'LSB', 'USB', 'CW-L', 'CW-U', 'NFM', 'WFM', 'WFM-Stereo', 'WFM-OIRT', 'ADS-B', 'LRPT']
const squelchLevel = ref(-150)
const filterShape = ref('normal')

// Local ref for demod, synced with prop
const selectedDemod = ref(props.demod)
watch(() => props.demod, (val) => { if (val) selectedDemod.value = val })
watch(selectedDemod, (val) => {
  if (!val || val === props.demod) return
  emit('update:demod', val)
})

// One-time sync from backend status (on page load / reconnect)
let synced = false
watch(statusLoaded, (loaded) => {
  if (!loaded || synced) return
  synced = true
  const s = status.value
  squelchLevel.value = s.SquelchLevel
  filterShape.value = (s.FilterShape || 'Normal').toLowerCase()
}, { immediate: true })

const squelchSlider = computed({
  get: () => [squelchLevel.value],
  set: (val: number[]) => { squelchLevel.value = val[0] },
})

async function onFilterLowChange(e: Event) {
  const val = parseInt((e.target as HTMLInputElement).value)
  if (isNaN(val)) return
  emit('update:filter', val, props.filterHigh)
}

async function onFilterHighChange(e: Event) {
  const val = parseInt((e.target as HTMLInputElement).value)
  if (isNaN(val)) return
  emit('update:filter', props.filterLow, val)
}

async function onFilterPreset(preset: string) {
  try {
    await api.setFilterPreset(preset)
  } catch (e) {
    console.error('Set filter preset failed:', e)
  }
}

async function onFilterShapeChange() {
  try {
    await api.setFilterShape(filterShape.value)
  } catch (e) {
    console.error('Set filter shape failed:', e)
  }
}

const onSquelchChange = debounce(async () => {
  try {
    await api.setSquelch(squelchLevel.value)
  } catch (e) {
    console.error('Set squelch failed:', e)
  }
}, 150)
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

.select-display {
  flex: 1;
  text-align: left;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
