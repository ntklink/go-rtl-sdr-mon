import { ref } from 'vue'

// Translation messages for supported locales
const messages: Record<string, Record<string, string>> = {
  'zh-CN': {
    // Top bar
    'top.signal': '信号',
    'top.squelch': '静噪',
    'top.squelchOpen': '开',
    'top.squelchClosed': '静音',
    'top.sampleRate': '采样率',
    'top.set': '设置',

    // Tabs
    'tab.receiver': '接收机',
    'tab.gain': '增益/FFT',
    'tab.audio': '音频',

    // Receiver panel
    'rx.title': '接收机选项',
    'rx.demod': '解调模式',
    'rx.filterLow': '滤波器低截止 (Hz)',
    'rx.filterHigh': '滤波器高截止 (Hz)',
    'rx.filterPreset': '滤波器预设',
    'rx.filterShape': '滤波器形状',
    'rx.squelch': '静噪电平 (dBFS)',
    'rx.squelchOff': '关闭',
    'rx.shape.soft': 'Soft',
    'rx.shape.normal': 'Normal',
    'rx.shape.sharp': 'Sharp',

    // Gain panel
    'gain.title': '增益与 FFT',
    'gain.autoGain': 'SDR 自动增益',
    'gain.agcPreset': 'AGC 预设',
    'gain.manual': '手动增益',
    'gain.ppm': '频率校准 (ppm)',
    'gain.fftAvg': 'FFT 平滑',
    'gain.fftSize': 'FFT 大小',
    'gain.fftRate': 'FFT 刷新率 (fps)',
    'gain.fftMaxHold': 'FFT Max-Hold',
    'gain.spectrumBins': '频谱点数',
    'gain.spectrumFull': '全量',

    // Audio panel
    'audio.title': '音频',
    'audio.play': '▶ 播放',
    'audio.stop': '⏸ 停止',
    'audio.volume': '音量',
    'audio.playing': '播放中',
    'audio.stopped': '已停止',

    // Frequency control
    'freq.unit': 'Hz',

    // Device selector
    'device.select': '选择设备...',
    'device.refresh': '刷新设备列表',

    // ADS-B
    'adsb.tab': 'ADS-B',
    'adsb.aircraft': '飞机',
    'adsb.callsign': '航班号',
    'adsb.icao': 'ICAO',
    'adsb.altitude': '高度',
    'adsb.speed': '速度',
    'adsb.track': '航向',
    'adsb.vRate': '升降率',
    'adsb.position': '位置',
    'adsb.count': '飞机数',
    'adsb.rxPos': '接收机位置',
    'adsb.lat': '纬度',
    'adsb.lon': '经度',
    'adsb.setRxPos': '设置',
    'adsb.noData': '暂无飞机数据',
    'adsb.geoLocate': '获取浏览器定位',
    'adsb.geoUnsupported': '浏览器不支持定位功能',
    'adsb.geoDenied': '定位权限被拒绝',
    'adsb.geoUnavailable': '无法获取位置信息',
    'adsb.geoTimeout': '定位超时',
    'adsb.detected': '检测',
    'adsb.valid': '有效',
    'adsb.tip': '提示：确保采样率为 2 MHz，频率为 1090 MHz，增益调高或开启自动增益。需要 1090 MHz 天线。',
  },

  'en': {
    'top.signal': 'Signal',
    'top.squelch': 'Squelch',
    'top.squelchOpen': 'Open',
    'top.squelchClosed': 'Muted',
    'top.sampleRate': 'Rate',
    'top.set': 'Set',

    'tab.receiver': 'Receiver',
    'tab.gain': 'Gain/FFT',
    'tab.audio': 'Audio',

    'rx.title': 'Receiver Options',
    'rx.demod': 'Demodulator',
    'rx.filterLow': 'Filter Low (Hz)',
    'rx.filterHigh': 'Filter High (Hz)',
    'rx.filterPreset': 'Filter Preset',
    'rx.filterShape': 'Filter Shape',
    'rx.squelch': 'Squelch Level (dBFS)',
    'rx.squelchOff': 'Off',
    'rx.shape.soft': 'Soft',
    'rx.shape.normal': 'Normal',
    'rx.shape.sharp': 'Sharp',

    'gain.title': 'Gain & FFT',
    'gain.autoGain': 'SDR Auto Gain',
    'gain.agcPreset': 'AGC Preset',
    'gain.manual': 'Manual Gain',
    'gain.ppm': 'Freq Correction (ppm)',
    'gain.fftAvg': 'FFT Averaging',
    'gain.fftSize': 'FFT Size',
    'gain.fftRate': 'FFT Rate (fps)',
    'gain.fftMaxHold': 'FFT Max-Hold',
    'gain.spectrumBins': 'Spectrum Bins',
    'gain.spectrumFull': 'Full',

    'audio.title': 'Audio',
    'audio.play': '▶ Play',
    'audio.stop': '⏸ Stop',
    'audio.volume': 'Volume',
    'audio.playing': 'Playing',
    'audio.stopped': 'Stopped',

    'freq.unit': 'Hz',

    'device.select': 'Select device...',
    'device.refresh': 'Refresh device list',

    // ADS-B
    'adsb.tab': 'ADS-B',
    'adsb.aircraft': 'Aircraft',
    'adsb.callsign': 'Callsign',
    'adsb.icao': 'ICAO',
    'adsb.altitude': 'Altitude',
    'adsb.speed': 'Speed',
    'adsb.track': 'Track',
    'adsb.vRate': 'V/S',
    'adsb.position': 'Position',
    'adsb.count': 'Aircraft',
    'adsb.rxPos': 'Receiver Position',
    'adsb.lat': 'Latitude',
    'adsb.lon': 'Longitude',
    'adsb.setRxPos': 'Set',
    'adsb.noData': 'No aircraft data',
    'adsb.geoLocate': 'Get browser location',
    'adsb.geoUnsupported': 'Geolocation not supported',
    'adsb.geoDenied': 'Location permission denied',
    'adsb.geoUnavailable': 'Position unavailable',
    'adsb.geoTimeout': 'Location request timed out',
    'adsb.detected': 'Detected',
    'adsb.valid': 'Valid',
    'adsb.tip': 'Tip: Ensure sample rate is 2 MHz, frequency is 1090 MHz, gain is high or auto. A 1090 MHz antenna is required.',
  },
}

const locale = ref(typeof localStorage !== 'undefined' ? localStorage.getItem('sdr-locale') || 'zh-CN' : 'zh-CN')

function setLocale(l: string) {
  locale.value = l
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem('sdr-locale', l)
  }
}

export function useI18n() {
  const t = (key: string): string => {
    return messages[locale.value]?.[key] || messages['en']?.[key] || key
  }

  return { t, locale, setLocale }
}
