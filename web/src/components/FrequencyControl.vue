<template>
  <div class="frequency-control">
    <span class="freq-label">{{ t('freq.label') }}</span>
    <div class="freq-display" @wheel.prevent="onWheel" :title="t('freq.scrollHint')">
      <input v-if="editing" ref="editInput" v-model="inputFreq" type="text" class="freq-edit-input"
        @keyup.enter="applyFreq" @keyup.esc="cancelEdit" @blur="applyFreq" @wheel.stop.prevent />
      <span v-else class="freq-value" @click="startEdit" :title="t('freq.clickEdit')">{{ formattedFreq }}</span>
      <button class="freq-unit-btn" @click="cycleUnit" :title="t('freq.cycleUnit')">{{ freqUnit }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, watch } from 'vue'
import { useI18n } from '../composables/useI18n'

const { t } = useI18n()

const props = defineProps<{
  frequency: number
  demod?: string
}>()

const emit = defineEmits<{
  'update:frequency': [value: number]
}>()

const UNITS = ['Hz', 'kHz', 'MHz'] as const
type Unit = typeof UNITS[number]

const inputFreq = ref('')
const freqUnitSelect = ref<Unit>('MHz')
const editing = ref(false)
const editInput = ref<HTMLInputElement | null>(null)

// Maps demod mode to the most natural display unit.
function defaultUnitForDemod(demod: string | undefined): Unit {
  if (!demod) return 'MHz'
  // VHF/UHF modes — frequencies typically in MHz
  if (['NFM', 'WFM', 'WFM-Stereo', 'WFM-OIRT', 'ADS-B', 'NOAA'].includes(demod)) return 'MHz'
  // AM/SSB/CW — AM broadcast (kHz) and HF bands read naturally in kHz
  if (['AM', 'AM-Sync', 'LSB', 'USB', 'CW-L', 'CW-U'].includes(demod)) return 'kHz'
  return 'MHz'
}

// Auto-switch unit when demod changes (does not override while editing).
watch(() => props.demod, (d) => {
  if (editing.value) return
  const u = defaultUnitForDemod(d)
  if (u !== freqUnitSelect.value) freqUnitSelect.value = u
}, { immediate: true })

const freqUnit = computed(() => freqUnitSelect.value)

const formattedFreq = computed(() => {
  const f = props.frequency
  switch (freqUnitSelect.value) {
    case 'MHz': return (f / 1e6).toFixed(4)
    case 'kHz': return (f / 1e3).toFixed(2)
    default: return f.toString()
  }
})

function startEdit() {
  inputFreq.value = formattedFreq.value
  editing.value = true
  nextTick(() => {
    editInput.value?.focus()
    editInput.value?.select()
  })
}

function applyFreq() {
  if (!editing.value) return
  editing.value = false
  const val = parseFloat(inputFreq.value)
  if (isNaN(val)) return
  let hz = val
  switch (freqUnitSelect.value) {
    case 'kHz': hz = val * 1000; break
    case 'MHz': hz = val * 1_000_000; break
  }
  hz = Math.max(0, Math.min(2_200_000_000, Math.round(hz)))
  if (hz !== props.frequency) emit('update:frequency', hz)
}

function cancelEdit() {
  editing.value = false
}

function cycleUnit() {
  if (editing.value) return
  const idx = UNITS.indexOf(freqUnitSelect.value)
  freqUnitSelect.value = UNITS[(idx + 1) % UNITS.length]
}

// Mouse-wheel tuning: Shift = 10× larger step. Step scales with the active unit.
function onWheel(e: WheelEvent) {
  if (editing.value) return
  let step = 100
  switch (freqUnitSelect.value) {
    case 'kHz': step = 1_000; break
    case 'MHz': step = 100_000; break
  }
  if (e.shiftKey) step *= 10
  const newFreq = Math.max(0, Math.min(2_200_000_000, props.frequency + (e.deltaY < 0 ? step : -step)))
  if (newFreq !== props.frequency) emit('update:frequency', newFreq)
}
</script>

<style scoped>
.frequency-control {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 16px;
  background: #111;
}

.freq-label {
  font-size: 10px;
  color: #666;
  text-transform: uppercase;
  letter-spacing: 1px;
}

.freq-display {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-family: 'Courier New', monospace;
  font-weight: bold;
  color: #00ff88;
  text-shadow: 0 0 10px rgba(0, 255, 136, 0.5);
  cursor: pointer;
  user-select: none;
  min-width: 200px;
}

.freq-value {
  font-size: 28px;
}

.freq-edit-input {
  font-family: 'Courier New', monospace;
  font-size: 28px;
  font-weight: bold;
  color: #00ff88;
  background: transparent;
  border: none;
  border-bottom: 1px solid #00ff88;
  outline: none;
  width: 180px;
  padding: 0;
}

.freq-unit-btn {
  font-size: 14px;
  color: #888;
  background: transparent;
  border: 1px solid #333;
  border-radius: 3px;
  padding: 2px 8px;
  cursor: pointer;
  font-family: inherit;
}

.freq-unit-btn:hover {
  color: #00ff88;
  border-color: #00ff88;
}
</style>
