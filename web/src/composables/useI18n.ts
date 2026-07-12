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
    'freq.label': '频率',
    'freq.clickEdit': '点击编辑频率',
    'freq.cycleUnit': '切换单位',
    'freq.scrollHint': '滚轮调谐（Shift 加大步进）',

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
    'adsb.showHistory': '历史',
    'adsb.noHistory': '暂无历史记录',
    'adsb.geoLocate': '获取浏览器定位',
    'adsb.geoUnsupported': '浏览器不支持定位功能',
    'adsb.geoDenied': '定位权限被拒绝',
    'adsb.geoUnavailable': '无法获取位置信息',
    'adsb.geoTimeout': '定位超时',
    'adsb.detected': '检测',
    'adsb.valid': '有效',
    'adsb.accepted': '采纳',
    'adsb.tip': '提示：确保采样率为 2 MHz，频率为 1090 MHz，增益调高或开启自动增益。需要 1090 MHz 天线。',

    // Meteor-M LRPT
    'lrpt.tab': 'LRPT',
    'lrpt.title': 'Meteor-M 气象卫星 (LRPT)',
    'lrpt.satellite': '卫星',
    'lrpt.signal': '信号质量',
    'lrpt.locked': '已锁定',
    'lrpt.unlocked': '未锁定',
    'lrpt.freqOffset': '频偏',
    'lrpt.frames': '帧',
    'lrpt.packets': '包',
    'lrpt.rs': 'RS 纠错',
    'lrpt.channel': '通道',
    'lrpt.constellation': '星座图',
    'lrpt.reset': '清除图像',
    'lrpt.save': '保存图像',
    'lrpt.noImage': '暂无图像数据',
    'lrpt.noSignal': '未锁定信号 — 请确认卫星正在过境、频率正确、增益足够。LRPT 需要 137 MHz 天线（V-dipole/QFH）。',
    'lrpt.noFrames': '已锁定载波但未解出帧 — 信号可能太弱（误码率过高），尝试提高增益或等待卫星升高。',
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
    'freq.label': 'Freq',
    'freq.clickEdit': 'Click to edit frequency',
    'freq.cycleUnit': 'Cycle unit',
    'freq.scrollHint': 'Scroll to tune (Shift for larger step)',

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
    'adsb.showHistory': 'History',
    'adsb.noHistory': 'No history records',
    'adsb.geoLocate': 'Get browser location',
    'adsb.geoUnsupported': 'Geolocation not supported',
    'adsb.geoDenied': 'Location permission denied',
    'adsb.geoUnavailable': 'Position unavailable',
    'adsb.geoTimeout': 'Location request timed out',
    'adsb.detected': 'Detected',
    'adsb.valid': 'Valid',
    'adsb.accepted': 'Accepted',
    'adsb.tip': 'Tip: Ensure sample rate is 2 MHz, frequency is 1090 MHz, gain is high or auto. A 1090 MHz antenna is required.',

    // Meteor-M LRPT
    'lrpt.tab': 'LRPT',
    'lrpt.title': 'Meteor-M Weather Sat (LRPT)',
    'lrpt.satellite': 'Satellite',
    'lrpt.signal': 'Signal quality',
    'lrpt.locked': 'Locked',
    'lrpt.unlocked': 'Unlocked',
    'lrpt.freqOffset': 'Freq offset',
    'lrpt.frames': 'Frames',
    'lrpt.packets': 'Packets',
    'lrpt.rs': 'RS fixes',
    'lrpt.channel': 'Channel',
    'lrpt.constellation': 'Constellation',
    'lrpt.reset': 'Clear Image',
    'lrpt.save': 'Save Image',
    'lrpt.noImage': 'No image data',
    'lrpt.noSignal': 'No carrier lock — make sure a satellite is passing, the frequency is right and gain is sufficient. LRPT needs a 137 MHz antenna (V-dipole/QFH).',
    'lrpt.noFrames': 'Carrier locked but no frames decode — signal may be too weak (BER too high). Increase gain or wait for a higher elevation.',
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
