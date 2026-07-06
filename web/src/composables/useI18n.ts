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
