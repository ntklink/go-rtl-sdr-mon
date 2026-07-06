<template>
  <div class="panel gain-panel">
    <h3>{{ t('gain.title') }}</h3>

    <div class="control-group">
      <div class="switch-row">
        <label>{{ t('gain.autoGain') }}</label>
        <SwitchRoot v-model="autoGain" class="reka-switch-root" @update:model-value="onAutoGainChange">
          <SwitchThumb class="reka-switch-thumb" />
        </SwitchRoot>
      </div>
    </div>

    <div class="control-group">
      <label>{{ t('gain.agcPreset') }}</label>
      <SelectRoot v-model="agcPreset" @update:model-value="onAGCPresetChange">
        <SelectTrigger class="reka-select-trigger">
          <span class="select-display">{{ agcPreset || '...' }}</span>
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M6 9l6 6 6-6" />
          </svg>
        </SelectTrigger>
        <SelectPortal>
          <SelectContent class="reka-select-content" position="popper" :side-offset="4">
            <SelectItem value="off" class="reka-select-item">Off</SelectItem>
            <SelectItem value="slow" class="reka-select-item">Slow</SelectItem>
            <SelectItem value="medium" class="reka-select-item">Medium</SelectItem>
            <SelectItem value="fast" class="reka-select-item">Fast</SelectItem>
          </SelectContent>
        </SelectPortal>
      </SelectRoot>
    </div>

    <div v-if="!autoGain" class="control-group">
      <label>{{ t('gain.manual') }}</label>
      <div class="slider-row">
        <SliderRoot
          v-model="gainSlider"
          :min="0"
          :max="Math.max(0, gains.length - 1)"
          :step="1"
          class="reka-slider-root"
          @update:model-value="onGainChange"
        >
          <SliderTrack class="reka-slider-track">
            <SliderRange class="reka-slider-range" />
          </SliderTrack>
          <SliderThumb class="reka-slider-thumb" />
        </SliderRoot>
        <span class="value-display">
          {{ gains.length > 0 ? (gains[gainIndex] / 10).toFixed(1) + ' dB' : 'N/A' }}
        </span>
      </div>
    </div>

    <div class="control-group">
      <label>{{ t('gain.ppm') }}</label>
      <input
        type="number"
        v-model.number="ppm"
        class="input"
        step="1"
        @change="onPpmChange"
      />
    </div>

    <div class="control-group">
      <label>{{ t('gain.fftAvg') }}</label>
      <div class="slider-row">
        <SliderRoot
          v-model="avgSlider"
          :min="0"
          :max="0.95"
          :step="0.05"
          class="reka-slider-root"
          @update:model-value="onAvgChange"
        >
          <SliderTrack class="reka-slider-track">
            <SliderRange class="reka-slider-range" />
          </SliderTrack>
          <SliderThumb class="reka-slider-thumb" />
        </SliderRoot>
        <span class="value-display">{{ spectrumAvg.toFixed(2) }}</span>
      </div>
    </div>

    <div class="control-group">
      <label>{{ t('gain.fftSize') }}</label>
      <SelectRoot v-model="fftSizeStr" @update:model-value="onFFTSizeChange">
        <SelectTrigger class="reka-select-trigger">
          <span class="select-display">{{ fftSizeStr || '...' }}</span>
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M6 9l6 6 6-6" />
          </svg>
        </SelectTrigger>
        <SelectPortal>
          <SelectContent class="reka-select-content" position="popper" :side-offset="4">
            <SelectItem v-for="s in fftSizes" :key="s" :value="String(s)" class="reka-select-item">{{ s }}</SelectItem>
          </SelectContent>
        </SelectPortal>
      </SelectRoot>
    </div>

    <div class="control-group">
      <label>{{ t('gain.spectrumBins') }}</label>
      <SelectRoot v-model="spectrumBinsStr" @update:model-value="onSpectrumBinsChange">
        <SelectTrigger class="reka-select-trigger">
          <span class="select-display">{{ spectrumBinsStr === '0' ? t('gain.spectrumFull') : spectrumBinsStr }}</span>
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M6 9l6 6 6-6" />
          </svg>
        </SelectTrigger>
        <SelectPortal>
          <SelectContent class="reka-select-content" position="popper" :side-offset="4">
            <SelectItem v-for="b in spectrumBinsOptions" :key="b" :value="String(b)" class="reka-select-item">{{ b === 0 ? t('gain.spectrumFull') : b }}</SelectItem>
          </SelectContent>
        </SelectPortal>
      </SelectRoot>
    </div>

    <div class="control-group">
      <label>{{ t('gain.fftRate') }}</label>
      <div class="slider-row">
        <SliderRoot
          v-model="fftRateSlider"
          :min="1"
          :max="60"
          :step="1"
          class="reka-slider-root"
          @update:model-value="onFFTRateChange"
        >
          <SliderTrack class="reka-slider-track">
            <SliderRange class="reka-slider-range" />
          </SliderTrack>
          <SliderThumb class="reka-slider-thumb" />
        </SliderRoot>
        <span class="value-display">{{ fftRate.toFixed(0) }} fps</span>
      </div>
    </div>

    <div class="control-group">
      <div class="switch-row">
        <label>{{ t('gain.fftMaxHold') }}</label>
        <SwitchRoot v-model="fftMaxHold" class="reka-switch-root" @update:model-value="onFFTMaxHoldChange">
          <SwitchThumb class="reka-switch-thumb" />
        </SwitchRoot>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { SwitchRoot, SwitchThumb } from 'reka-ui'
import { SliderRoot, SliderTrack, SliderRange, SliderThumb } from 'reka-ui'
import { SelectRoot, SelectTrigger, SelectContent, SelectItem, SelectPortal } from 'reka-ui'
import { useApi } from '../composables/useApi'
import { useI18n } from '../composables/useI18n'
import { useStatus } from '../composables/useStatus'
import { spectrumBins } from '../composables/useWaterfall'
import { debounce } from '../composables/useDebounce'

const api = useApi()
const { t } = useI18n()
const { status, statusLoaded } = useStatus()

const autoGain = ref(true)
const gainIndex = ref(0)
const gains = ref<number[]>([])
const ppm = ref(0)
const spectrumAvg = ref(0.3)
const agcPreset = ref('medium')
const fftSizes = [1024, 2048, 4096, 8192, 16384]
const fftSizeStr = ref('8192')
const fftRate = ref(25)
const fftMaxHold = ref(false)
const spectrumBinsOptions = [0, 256, 512, 1024, 2048, 4096]
const spectrumBinsStr = ref(String(spectrumBins.value))

// One-time sync from backend status (on page load / reconnect)
let synced = false
function syncFromStatus() {
  if (synced || !statusLoaded.value) return
  synced = true
  const s = status.value
  autoGain.value = s.AutoGain
  ppm.value = s.FreqCorrection
  spectrumAvg.value = s.SpectrumAvg
  fftSizeStr.value = String(s.FFTSize)
  fftRate.value = s.FFTRate
  fftMaxHold.value = s.FFTMaxHold
  agcPreset.value = (s.AGCPreset || 'Medium').toLowerCase()
  syncGainIndex()
}

// Find closest gain index from backend Gain value
function syncGainIndex() {
  const s = status.value
  if (gains.value.length > 0 && s.Gain > 0) {
    let bestIdx = 0
    let bestDiff = Math.abs(gains.value[0] - s.Gain)
    for (let i = 1; i < gains.value.length; i++) {
      const diff = Math.abs(gains.value[i] - s.Gain)
      if (diff < bestDiff) {
        bestDiff = diff
        bestIdx = i
      }
    }
    gainIndex.value = bestIdx
  }
}

// Sync immediately if status is already loaded, otherwise wait for it
watch(statusLoaded, () => syncFromStatus(), { immediate: true })
// If gains arrive after status sync, try gain index sync again
watch(gains, () => { if (synced) syncGainIndex() })

const gainSlider = computed({
  get: () => [gainIndex.value],
  set: (val: number[]) => { gainIndex.value = val[0] },
})

const avgSlider = computed({
  get: () => [spectrumAvg.value],
  set: (val: number[]) => { spectrumAvg.value = val[0] },
})

const fftRateSlider = computed({
  get: () => [fftRate.value],
  set: (val: number[]) => { fftRate.value = val[0] },
})

onMounted(async () => {
  try {
    const info = await api.getDeviceInfo()
    gains.value = info.Gains || []
  } catch (e) {
    console.error('Get device info failed:', e)
  }
})

async function onAutoGainChange() {
  try {
    await api.setAutoGain(autoGain.value)
  } catch (e) {
    console.error('Set auto gain failed:', e)
  }
}

const onGainChange = debounce(async () => {
  if (gains.value.length === 0) return
  const gain = gains.value[gainIndex.value]
  try {
    await api.setGain(gain)
  } catch (e) {
    console.error('Set gain failed:', e)
  }
}, 150)

const onAvgChange = debounce(async () => {
  try {
    await api.setSpectrumAvg(spectrumAvg.value)
  } catch (e) {
    console.error('Set spectrum avg failed:', e)
  }
}, 150)

const onFFTRateChange = debounce(async () => {
  try {
    await api.setFFTRate(fftRate.value)
  } catch (e) {
    console.error('Set FFT rate failed:', e)
  }
}, 150)

async function onPpmChange() {
  try {
    await api.setFreqCorrection(ppm.value)
  } catch (e) {
    console.error('Set ppm failed:', e)
  }
}

async function onAGCPresetChange() {
  try {
    await api.setAGCPreset(agcPreset.value)
  } catch (e) {
    console.error('Set AGC preset failed:', e)
  }
}

async function onFFTSizeChange() {
  try {
    await api.setFFTSize(parseInt(fftSizeStr.value))
  } catch (e) {
    console.error('Set FFT size failed:', e)
  }
}

function onSpectrumBinsChange() {
  spectrumBins.value = parseInt(spectrumBinsStr.value)
}

async function onFFTMaxHoldChange() {
  try {
    await api.setFFTMaxHold(fftMaxHold.value)
  } catch (e) {
    console.error('Set FFT max-hold failed:', e)
  }
}
</script>

<style scoped>
.gain-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.switch-row label {
  cursor: pointer;
}

.slider-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.select-display {
  flex: 1;
  text-align: left;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
