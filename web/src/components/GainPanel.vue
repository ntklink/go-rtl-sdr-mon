<template>
  <div class="panel gain-panel">
    <h3>增益控制</h3>

    <div class="control-group">
      <div class="switch-row">
        <label>自动增益 (AGC)</label>
        <SwitchRoot v-model="autoGain" class="reka-switch-root" @update:model-value="onAutoGainChange">
          <SwitchThumb class="reka-switch-thumb" />
        </SwitchRoot>
      </div>
    </div>

    <div class="control-group">
      <label>AGC 预设</label>
      <SelectRoot v-model="agcPreset" @update:model-value="onAGCPresetChange">
        <SelectTrigger class="reka-select-trigger">
          <SelectValue placeholder="选择..." />
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
      <label>手动增益</label>
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
      <label>频率校准 (ppm)</label>
      <input
        type="number"
        v-model.number="ppm"
        class="input"
        step="1"
        @change="onPpmChange"
      />
    </div>

    <div class="control-group">
      <label>FFT 平滑</label>
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
      <label>FFT 大小</label>
      <SelectRoot v-model="fftSizeStr" @update:model-value="onFFTSizeChange">
        <SelectTrigger class="reka-select-trigger">
          <SelectValue placeholder="选择..." />
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
      <label>FFT 刷新率 (fps)</label>
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
        <label>FFT Max-Hold</label>
        <SwitchRoot v-model="fftMaxHold" class="reka-switch-root" @update:model-value="onFFTMaxHoldChange">
          <SwitchThumb class="reka-switch-thumb" />
        </SwitchRoot>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { SwitchRoot, SwitchThumb } from 'reka-ui'
import { SliderRoot, SliderTrack, SliderRange, SliderThumb } from 'reka-ui'
import { SelectRoot, SelectTrigger, SelectValue, SelectContent, SelectItem, SelectPortal } from 'reka-ui'
import { useApi } from '../composables/useApi'

const api = useApi()

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

async function onGainChange() {
  if (gains.value.length === 0) return
  const gain = gains.value[gainIndex.value]
  try {
    await api.setGain(gain)
  } catch (e) {
    console.error('Set gain failed:', e)
  }
}

async function onPpmChange() {
  try {
    await api.setFreqCorrection(ppm.value)
  } catch (e) {
    console.error('Set ppm failed:', e)
  }
}

async function onAvgChange() {
  try {
    await api.setSpectrumAvg(spectrumAvg.value)
  } catch (e) {
    console.error('Set spectrum avg failed:', e)
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

async function onFFTRateChange() {
  try {
    await api.setFFTRate(fftRate.value)
  } catch (e) {
    console.error('Set FFT rate failed:', e)
  }
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
</style>
