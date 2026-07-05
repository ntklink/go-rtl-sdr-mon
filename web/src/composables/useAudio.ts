import { ref, onUnmounted } from 'vue'

export function useAudio() {
  const isPlaying = ref(false)
  const volume = ref(0.8)
  const error = ref<string | null>(null)

  let ws: WebSocket | null = null
  let audioCtx: AudioContext | null = null
  let gainNode: GainNode | null = null
  let reconnectTimer: number | null = null
  let sampleRate = 48000
  let channels = 1

  // Buffer queue for smooth playback
  let nextPlayTime = 0
  const BUFFER_THRESHOLD = 0.05 // 50ms minimum buffer before playing

  function initAudio() {
    if (audioCtx) return
    audioCtx = new (window.AudioContext || (window as any).webkitAudioContext)()
    gainNode = audioCtx.createGain()
    gainNode.gain.value = volume.value
    gainNode.connect(audioCtx.destination)
    nextPlayTime = audioCtx.currentTime
  }

  function connect() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    ws = new WebSocket(`${proto}//${location.host}/api/ws/audio`)
    ws.binaryType = 'arraybuffer'

    ws.onmessage = (ev) => {
      if (typeof ev.data === 'string') return

      if (!audioCtx) {
        initAudio()
      }

      if (!audioCtx || !gainNode) return

      if (audioCtx.state === 'suspended') {
        audioCtx.resume()
      }

      const buf = new DataView(ev.data as ArrayBuffer)
      channels = buf.getUint8(0)
      const numSamples = buf.getUint32(1, true)
      const offset = 5

      // Create audio buffer
      const audioBuffer = audioCtx.createBuffer(channels, numSamples, sampleRate)

      if (channels === 1) {
        const ch0 = audioBuffer.getChannelData(0)
        for (let i = 0; i < numSamples; i++) {
          ch0[i] = buf.getInt16(offset + i * 2, true) / 32768
        }
      } else {
        const ch0 = audioBuffer.getChannelData(0)
        const ch1 = audioBuffer.getChannelData(1)
        for (let i = 0; i < numSamples; i++) {
          ch0[i] = buf.getInt16(offset + i * 4, true) / 32768
          ch1[i] = buf.getInt16(offset + i * 4 + 2, true) / 32768
        }
      }

      // Schedule playback
      const source = audioCtx.createBufferSource()
      source.buffer = audioBuffer
      source.connect(gainNode)

      const now = audioCtx.currentTime
      if (nextPlayTime < now + BUFFER_THRESHOLD) {
        nextPlayTime = now + BUFFER_THRESHOLD
      }

      source.start(nextPlayTime)
      nextPlayTime += audioBuffer.duration

      isPlaying.value = true
    }

    ws.onclose = () => {
      isPlaying.value = false
      reconnectTimer = window.setTimeout(() => connect(), 2000)
    }
    ws.onerror = () => {
      error.value = 'Audio connection error'
      ws?.close()
    }
  }

  function start() {
    error.value = null
    if (!audioCtx) {
      initAudio()
    }
    audioCtx?.resume()
    connect()
  }

  function stop() {
    isPlaying.value = false
    if (ws) {
      ws.onclose = null
      ws.close()
      ws = null
    }
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  function setVolume(v: number) {
    volume.value = v
    if (gainNode) {
      gainNode.gain.value = v
    }
  }

  onUnmounted(() => {
    stop()
    if (audioCtx) {
      audioCtx.close()
    }
  })

  return { isPlaying, volume, error, start, stop, setVolume }
}
