<template>
  <div class="app">
    <!-- Top bar: Device, Frequency, Status -->
    <div class="top-bar">
      <div class="top-left">
        <DeviceSelector />
        <FrequencyControl
          :frequency="status.CenterFreq"
          @update:frequency="setFrequency"
        />
      </div>
      <div class="top-right">
        <div class="status-bar">
          <div class="status-item">
            <span class="label">{{ t('top.signal') }}</span>
            <span class="value" :class="{ active: status.SquelchOpen }">
              {{ status.SignalLevel.toFixed(1) }} dBFS
            </span>
          </div>
          <div class="status-item">
            <span class="label">{{ t('top.sampleRate') }}</span>
            <span class="value">{{ (status.SampleRate / 1e6).toFixed(2) }} MHz</span>
          </div>
        </div>
        <button class="locale-btn" @click="toggleLocale" :title="locale === 'zh-CN' ? 'Switch to English' : '切换到中文'">
          {{ locale === 'zh-CN' ? 'CN' : 'EN' }}
        </button>
      </div>
    </div>

    <!-- Main area: Waterfall + tabbed side panels -->
    <div class="main-area">
      <div class="center-area">
        <Waterfall :center-freq="status.CenterFreq" :sample-rate="status.SampleRate" />
      </div>
      <div class="side-panels">
        <TabsRoot default-value="receiver" class="reka-tabs-root side-tabs">
          <TabsList class="reka-tabs-list">
            <TabsTrigger value="receiver" class="reka-tabs-trigger">{{ t('tab.receiver') }}</TabsTrigger>
            <TabsTrigger value="gain" class="reka-tabs-trigger">{{ t('tab.gain') }}</TabsTrigger>
            <TabsTrigger value="audio" class="reka-tabs-trigger">{{ t('tab.audio') }}</TabsTrigger>
          </TabsList>
          <TabsContent value="receiver" force-mount class="reka-tabs-content">
            <ReceiverPanel
              :demod="status.Demod"
              :filter-low="status.FilterLow"
              :filter-high="status.FilterHigh"
              @update:demod="setDemod"
              @update:filter="setFilter"
            />
          </TabsContent>
          <TabsContent value="gain" force-mount class="reka-tabs-content">
            <GainPanel />
          </TabsContent>
          <TabsContent value="audio" force-mount class="reka-tabs-content">
            <AudioPlayer />
          </TabsContent>
        </TabsRoot>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { TabsRoot, TabsList, TabsTrigger, TabsContent } from 'reka-ui'
import Waterfall from './components/Waterfall.vue'
import FrequencyControl from './components/FrequencyControl.vue'
import DeviceSelector from './components/DeviceSelector.vue'
import ReceiverPanel from './components/ReceiverPanel.vue'
import GainPanel from './components/GainPanel.vue'
import AudioPlayer from './components/AudioPlayer.vue'
import { useApi } from './composables/useApi'
import { useStatus } from './composables/useStatus'
import { useI18n } from './composables/useI18n'

const api = useApi()
const { status } = useStatus()
const { t, locale, setLocale } = useI18n()

function toggleLocale() {
  setLocale(locale.value === 'zh-CN' ? 'en' : 'zh-CN')
}

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
