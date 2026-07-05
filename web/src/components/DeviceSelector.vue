<template>
  <div class="device-selector">
    <SelectRoot v-model="selectedID" @update:model-value="onSelect">
      <SelectTrigger class="reka-select-trigger device-trigger">
        <SelectValue :placeholder="t('device.select')" />
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M6 9l6 6 6-6" />
        </svg>
      </SelectTrigger>
      <SelectPortal>
        <SelectContent class="reka-select-content" position="popper" :side-offset="4">
          <SelectItem
            v-for="d in devices"
            :key="d.id"
            :value="d.id"
            class="reka-select-item"
          >
            <span class="device-label">
              <span class="device-name">{{ d.name || d.product || d.id }}</span>
              <span v-if="d.active" class="device-badge active">●</span>
              <span v-else-if="d.open" class="device-badge open">○</span>
            </span>
            <span v-if="d.serial" class="device-serial">{{ d.serial }}</span>
          </SelectItem>
        </SelectContent>
      </SelectPortal>
    </SelectRoot>
    <button v-if="selectedID" class="refresh-btn" @click="refresh" :title="t('device.refresh')">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M23 4v6h-6M1 20v-6h6" />
        <path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15" />
      </svg>
    </button>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { SelectRoot, SelectTrigger, SelectValue, SelectContent, SelectItem, SelectPortal } from 'reka-ui'
import { useApi } from '../composables/useApi'
import { useI18n } from '../composables/useI18n'

const api = useApi()
const { t } = useI18n()

interface DeviceItem {
  id: string
  driver: string
  index: number
  name: string
  manufacturer: string
  product: string
  serial: string
  open: boolean
  active: boolean
}

const devices = ref<DeviceItem[]>([])
const selectedID = ref('')

async function refresh() {
  try {
    const resp = await api.listDevices()
    devices.value = resp.devices || []
    const active = devices.value.find(d => d.active)
    if (active) {
      selectedID.value = active.id
    }
  } catch (e) {
    console.error('List devices failed:', e)
  }
}

async function onSelect() {
  if (!selectedID.value) return
  try {
    await api.selectDevice(selectedID.value)
    // Refresh to update open/active status
    await refresh()
  } catch (e) {
    console.error('Select device failed:', e)
  }
}

onMounted(() => {
  refresh()
})
</script>

<style scoped>
.device-selector {
  display: flex;
  align-items: center;
  gap: 6px;
}

.device-trigger {
  min-width: 220px;
}

.device-label {
  display: flex;
  align-items: center;
  gap: 6px;
}

.device-name {
  flex: 1;
}

.device-badge {
  font-size: 10px;
}

.device-badge.active {
  color: #00ff88;
}

.device-badge.open {
  color: #888;
}

.device-serial {
  font-size: 10px;
  color: #666;
  margin-left: 8px;
}

.refresh-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  background: #1a1a2e;
  border: 1px solid #333;
  color: #888;
  border-radius: 4px;
  cursor: pointer;
  flex-shrink: 0;
}

.refresh-btn:hover {
  color: #fff;
  border-color: #0066cc;
}
</style>
