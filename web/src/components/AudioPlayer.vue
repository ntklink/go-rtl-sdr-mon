<template>
  <div class="panel audio-panel">
    <h3>{{ t('audio.title') }}</h3>

    <div class="audio-controls">
      <button class="play-btn" :class="{ playing: isPlaying }" @click="togglePlay">
        {{ isPlaying ? t('audio.stop') : t('audio.play') }}
      </button>

      <div class="volume-control">
        <label>{{ t('audio.volume') }}</label>
        <SliderRoot :model-value="[volume]" @update:model-value="onVolumeChange" :min="0" :max="1" :step="0.01"
          class="reka-slider-root">
          <SliderTrack class="reka-slider-track">
            <SliderRange class="reka-slider-range" />
          </SliderTrack>
          <SliderThumb class="reka-slider-thumb" />
        </SliderRoot>
        <span class="value-display">{{ (volume * 100).toFixed(0) }}%</span>
      </div>
    </div>

    <div v-if="error" class="error">{{ error }}</div>

    <div class="audio-status">
      <span class="status-dot" :class="{ active: isPlaying }"></span>
      {{ isPlaying ? t('audio.playing') : t('audio.stopped') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { SliderRoot, SliderTrack, SliderRange, SliderThumb } from 'reka-ui'
import { useAudio } from '../composables/useAudio'
import { useI18n } from '../composables/useI18n'

const { isPlaying, volume, error, start, stop, setVolume } = useAudio()
const { t } = useI18n()

function togglePlay() {
  if (isPlaying.value) {
    stop()
  } else {
    start()
  }
}

function onVolumeChange(val: number[] | undefined) {
  if (val && val.length > 0) {
    setVolume(val[0])
  }
}
</script>

<style scoped>
.audio-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.audio-controls {
  display: flex;
  align-items: center;
  gap: 12px;
}

.play-btn {
  padding: 8px 16px;
  background: #0066cc;
  border: none;
  color: #fff;
  border-radius: 4px;
  cursor: pointer;
  font-size: 14px;
  font-weight: bold;
  min-width: 90px;
}

.play-btn:hover {
  background: #0080ff;
}

.play-btn.playing {
  background: #cc3300;
}

.play-btn.playing:hover {
  background: #ff4400;
}

.volume-control {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
}

.volume-control label {
  white-space: nowrap;
  font-size: 12px;
}

.error {
  color: #ff4444;
  font-size: 12px;
}

.audio-status {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: #888;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #555;
}

.status-dot.active {
  background: #00ff88;
  box-shadow: 0 0 6px #00ff88;
}
</style>
