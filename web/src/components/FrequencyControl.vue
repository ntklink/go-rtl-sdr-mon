<template>
  <div class="frequency-control">
    <div class="freq-display">
      <span class="freq-value">{{ formattedFreq }}</span>
      <span class="freq-unit">{{ freqUnit }}</span>
    </div>
    <div class="freq-input-group">
      <input
        v-model="inputFreq"
        type="number"
        class="freq-input"
        placeholder="Hz"
        @keyup.enter="applyFreq"
      />
      <SelectRoot v-model="freqUnitSelect">
        <SelectTrigger class="freq-unit-trigger reka-select-trigger">
          <span class="select-display">{{ freqUnitSelect }}</span>
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M6 9l6 6 6-6" />
          </svg>
        </SelectTrigger>
        <SelectPortal>
          <SelectContent class="reka-select-content" position="popper" :side-offset="4">
            <SelectItem value="Hz" class="reka-select-item">Hz</SelectItem>
            <SelectItem value="kHz" class="reka-select-item">kHz</SelectItem>
            <SelectItem value="MHz" class="reka-select-item">MHz</SelectItem>
          </SelectContent>
        </SelectPortal>
      </SelectRoot>
      <button @click="applyFreq" class="btn-apply">{{ t('top.set') }}</button>
    </div>
    <div class="freq-step-buttons">
      <button @click="stepFreq(-100000)">-100k</button>
      <button @click="stepFreq(-10000)">-10k</button>
      <button @click="stepFreq(-1000)">-1k</button>
      <button @click="stepFreq(1000)">+1k</button>
      <button @click="stepFreq(10000)">+10k</button>
      <button @click="stepFreq(100000)">+100k</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { SelectRoot, SelectTrigger, SelectValue, SelectContent, SelectItem, SelectPortal } from 'reka-ui'
import { useI18n } from '../composables/useI18n'

const { t } = useI18n()

const props = defineProps<{
  frequency: number
}>()

const emit = defineEmits<{
  'update:frequency': [value: number]
}>()

const inputFreq = ref('')
const freqUnitSelect = ref('Hz')

const freqUnit = computed(() => freqUnitSelect.value)

const formattedFreq = computed(() => {
  const f = props.frequency
  if (f >= 1e6) return (f / 1e6).toFixed(4)
  if (f >= 1e3) return (f / 1e3).toFixed(2)
  return f.toString()
})

function applyFreq() {
  const val = parseFloat(inputFreq.value)
  if (isNaN(val)) return
  let hz = val
  switch (freqUnitSelect.value) {
    case 'kHz': hz = val * 1000; break
    case 'MHz': hz = val * 1000000; break
  }
  hz = Math.max(0, Math.min(2200000000, Math.round(hz)))
  emit('update:frequency', hz)
  inputFreq.value = ''
}

function stepFreq(delta: number) {
  const newFreq = Math.max(0, Math.min(2200000000, props.frequency + delta))
  emit('update:frequency', newFreq)
}
</script>

<style scoped>
.frequency-control {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 8px 16px;
  background: #111;
}

.freq-display {
  font-family: 'Courier New', monospace;
  font-size: 28px;
  font-weight: bold;
  color: #00ff88;
  text-shadow: 0 0 10px rgba(0, 255, 136, 0.5);
  min-width: 200px;
}

.freq-unit {
  font-size: 14px;
  color: #666;
  margin-left: 4px;
}

.freq-input-group {
  display: flex;
  gap: 4px;
}

.freq-input {
  width: 120px;
  padding: 4px 8px;
  background: #1a1a2e;
  border: 1px solid #333;
  color: #fff;
  border-radius: 3px;
  font-size: 13px;
}

.freq-unit-trigger {
  width: auto;
  min-width: 70px;
}

.btn-apply {
  padding: 4px 12px;
  background: #0066cc;
  border: none;
  color: #fff;
  border-radius: 3px;
  cursor: pointer;
  font-size: 13px;
}

.btn-apply:hover {
  background: #0080ff;
}

.freq-step-buttons {
  display: flex;
  gap: 4px;
}

.freq-step-buttons button {
  padding: 4px 8px;
  background: #222;
  border: 1px solid #333;
  color: #ccc;
  border-radius: 3px;
  cursor: pointer;
  font-size: 12px;
}

.freq-step-buttons button:hover {
  background: #333;
}
</style>
