const API_BASE = '/api'

async function postJSON(path: string, body: any): Promise<any> {
  const resp = await fetch(`${API_BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!resp.ok) {
    throw new Error(`API error ${resp.status}: ${await resp.text()}`)
  }
  return resp.json()
}

async function getJSON(path: string): Promise<any> {
  const resp = await fetch(`${API_BASE}${path}`)
  if (!resp.ok) {
    throw new Error(`API error ${resp.status}: ${await resp.text()}`)
  }
  return resp.json()
}

export function useApi() {
  return {
    getDeviceInfo: () => getJSON('/device'),
    listDevices: () => getJSON('/devices'),
    selectDevice: (id: string) => postJSON('/device/select', { id }),
    getStatus: () => getJSON('/status'),
    setFrequency: (frequency: number) => postJSON('/frequency', { frequency }),
    setDemod: (demod: string) => postJSON('/demod', { demod }),
    setFilter: (low: number, high: number) => postJSON('/filter', { low, high }),
    setFilterOffset: (offset: number) => postJSON('/filter-offset', { offset }),
    setSquelch: (level: number) => postJSON('/squelch', { level }),
    setAGC: (enabled: boolean) => postJSON('/agc', { enabled }),
    setGain: (gain: number) => postJSON('/gain', { gain }),
    setAutoGain: (auto: boolean) => postJSON('/auto-gain', { auto }),
    setFreqCorrection: (ppm: number) => postJSON('/freq-correction', { ppm }),
    setSpectrumAvg: (avg: number) => postJSON('/spectrum-avg', { avg }),
    setFFTSize: (size: number) => postJSON('/fft-size', { size }),
    setFFTRate: (rate: number) => postJSON('/fft-rate', { rate }),
    setFFTMaxHold: (enabled: boolean) => postJSON('/fft-max-hold', { enabled }),
    setAGCPreset: (preset: string) => postJSON('/agc-preset', { preset }),
    setCWOffset: (offset: number) => postJSON('/cw-offset', { offset }),
    setFilterShape: (shape: string) => postJSON('/filter-shape', { shape }),
    setFilterPreset: (preset: string) => postJSON('/filter-preset', { preset }),
    setReceiverPosition: (latitude: number, longitude: number) => postJSON('/receiver-position', { latitude, longitude }),
    getADSBStats: () => getJSON('/adsb-stats'),
    getAircraft: () => getJSON('/aircraft'),
    getAircraftHistory: () => getJSON('/aircraft/history'),
    getAllAircraft: () => getJSON('/aircraft/all'),
    getNOAASatellites: () => getJSON('/noaa/satellites'),
    getAPTStats: () => getJSON('/apt-stats'),
    resetAPT: () => postJSON('/apt-reset', {}),
  }
}
