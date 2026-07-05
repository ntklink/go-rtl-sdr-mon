<template>
  <div class="app">
    <!-- Top bar: Frequency and status -->
    <div class="top-bar">
      <FrequencyControl
        :frequency="status.CenterFreq"
        @update:frequency="setFrequency"
      />
      <div class="status-bar">
        <div class="status-item">
          <span class="label">信号</span>
          <span class="value" :class="{ active: status.SquelchOpen }">
            {{ status.SignalLevel.toFixed(1) }} dBFS
          </span>
        </div>
        <div class="status-item">
          <span class="label">静噪</span>
          <span class="value" :class="{ active: status.SquelchOpen }">
            {{ status.SquelchOpen ? '开' : '静音' }}
          </span>
        </div>
        <div class="status-item">
          <span class="label">采样率</span>
          <span class="value">{{ (status.SampleRate / 1e6).toFixed(2) }} MHz</span>
        </div>
      </div>
    </div>

    <!-- Main area: Waterfall + side panels -->
    <div class="main-area">
      <div class="center-area">
        <Waterfall :center-freq="status.CenterFreq" :sample-rate="status.SampleRate" />
      </div>
      <div class="side-panels">
        <ReceiverPanel
          :demod="status.Demod"
          :filter-low="status.FilterLow"
          :filter-high="status.FilterHigh"
          @update:demod="setDemod"
          @update:filter="setFilter"
        />
        <GainPanel />
        <AudioPlayer />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import Waterfall from './components/Waterfall.vue'
import FrequencyControl from './components/FrequencyControl.vue'
import ReceiverPanel from './components/ReceiverPanel.vue'
import GainPanel from './components/GainPanel.vue'
import AudioPlayer from './components/AudioPlayer.vue'
import { useApi } from './composables/useApi'
import { useStatus } from './composables/useStatus'

const api = useApi()
const { status } = useStatus()

async function setFrequency(freq: number) {
  try {
    await api.setFrequency(freq)
  } catch (e) {
    console.error('Set frequency failed:', e)
  }
}

async function setDemod(demod: string) {
  try {
    await api.setDemod(demod)
  } catch (e) {
    console.error('Set demod failed:', e)
  }
}

async function setFilter(low: number, high: number) {
  try {
    await api.setFilter(low, high)
  } catch (e) {
    console.error('Set filter failed:', e)
  }
}

onMounted(async () => {
  try {
    const st = await api.getStatus()
    if (st.status) {
      status.value = { ...status.value, ...st.status }
    }
  } catch (e) {
    console.error('Get status failed:', e)
  }
})
</script>
