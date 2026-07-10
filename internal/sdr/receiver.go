package sdr

import (
	"log"
	"math"
	"sync"
	"time"

	"github.com/ntklink/go-rtl-sdr-mon/internal/adsb"
	"github.com/ntklink/go-rtl-sdr-mon/internal/demod"
	"github.com/ntklink/go-rtl-sdr-mon/internal/noaa"
)

// DemodType re-exports the demod type from the demod package for convenience.
type DemodType = demod.DemodType

// Demod constants (re-exported for API use). Values match gqrx DockRxOpt enum.
const (
	DemodOff       = demod.DemodOff       // 0
	DemodRaw       = demod.DemodRaw       // 1 - Raw I/Q
	DemodAM        = demod.DemodAM        // 2
	DemodAMSync    = demod.DemodAMSync    // 3
	DemodLSB       = demod.DemodLSB       // 4
	DemodUSB       = demod.DemodUSB       // 5
	DemodCWL       = demod.DemodCWL       // 6
	DemodCWU       = demod.DemodCWU       // 7
	DemodNFM       = demod.DemodNFM       // 8
	DemodWFM       = demod.DemodWFM       // 9
	DemodWFMStereo = demod.DemodWFMStereo // 10
	DemodWFMOirt   = demod.DemodWFMOirt   // 11
	DemodADSB      = demod.DemodADSB      // 12 - ADS-B
	DemodNOAA      = demod.DemodNOAA      // 13 - NOAA APT
)

// AudioRate is the standard audio output sample rate.
const AudioRate = 48000

// TargetQuadRate is the target quadrature rate after DDC decimation.
const TargetQuadRate = 240000

// Filter shape constants (match gqrx filter_shape enum).
const (
	FilterShapeSoft   = 0 // 50% transition band
	FilterShapeNormal = 1 // 20% transition band (gqrx default)
	FilterShapeSharp  = 2 // 10% transition band
)

// Filter preset constants.
const (
	FilterPresetWide   = 0
	FilterPresetNormal = 1
	FilterPresetNarrow = 2
)

// filterPresetTable defines the filter low/high cutoffs (Hz) for each
// demod mode and preset (Wide/Normal/Narrow). Values match gqrx exactly.
var filterPresetTable = map[DemodType][3][2]float64{
	DemodOff:       {{0, 0}, {0, 0}, {0, 0}},
	DemodRaw:       {{-15000, 15000}, {-5000, 5000}, {-1000, 1000}},
	DemodAM:        {{-10000, 10000}, {-5000, 5000}, {-2500, 2500}},
	DemodAMSync:    {{-10000, 10000}, {-5000, 5000}, {-2500, 2500}},
	DemodLSB:       {{-4000, -100}, {-2800, -100}, {-2400, -300}},
	DemodUSB:       {{100, 4000}, {100, 2800}, {300, 2400}},
	DemodCWL:       {{-1000, 1000}, {-250, 250}, {-100, 100}},
	DemodCWU:       {{-1000, 1000}, {-250, 250}, {-100, 100}},
	DemodNFM:       {{-10000, 10000}, {-5000, 5000}, {-2500, 2500}},
	DemodWFM:       {{-100000, 100000}, {-80000, 80000}, {-60000, 60000}},
	DemodWFMStereo: {{-100000, 100000}, {-80000, 80000}, {-60000, 60000}},
	DemodWFMOirt:   {{-100000, 100000}, {-80000, 80000}, {-60000, 60000}},
	DemodADSB:      {{-1000000, 1000000}, {-1000000, 1000000}, {-1000000, 1000000}},
	DemodNOAA:      {{-30000, 30000}, {-20000, 20000}, {-15000, 15000}},
}

// ReceiverConfig holds configuration for the receiver.
type ReceiverConfig struct {
	SampleRate     uint32
	CenterFreq     uint32
	FilterLow      float64 // Hz relative to center
	FilterHigh     float64 // Hz relative to center
	FilterOffset   float64 // Hz, offset from center frequency
	CWOffset       float64 // Hz, CW/BFO offset (gqrx default: 700)
	Demod          DemodType
	SquelchLevel   float64 // dBFS, -150 = open
	AGCOn          bool
	Gain           int // tenths of dB
	AutoGain       bool
	FreqCorrection int // ppm
}

// DefaultReceiverConfig returns a default configuration matching gqrx defaults.
func DefaultReceiverConfig() ReceiverConfig {
	return ReceiverConfig{
		SampleRate:     1800000,   // 1.8 MHz (gqrx default)
		CenterFreq:     102800000, // 102.8 MHz
		FilterLow:      -80000,    // ±80 kHz for WFM (NORMAL preset)
		FilterHigh:     80000,
		FilterOffset:   0,
		CWOffset:       700, // gqrx default CW offset (Hz)
		Demod:          DemodWFM,
		SquelchLevel:   -150, // open
		AGCOn:          true,
		AutoGain:       true,
		FreqCorrection: 0,
	}
}

// Receiver is the top-level receiver that orchestrates the DSP chain.
type Receiver struct {
	mu sync.Mutex

	source   SDRDevice
	spectrum *SpectrumFFT
	ddc      *DDC
	agc      *AGC // left/mono channel AGC
	agcR     *AGC // right-channel AGC (stereo), so L/R gains are independent

	demod     demod.Demodulator
	demodType DemodType

	// Filter
	filterLow    float64
	filterHigh   float64
	filterOffset float64
	filterShape  int     // FilterShapeSoft/Normal/Sharp
	cwOffset     float64 // CW/BFO offset in Hz

	// Bandpass filter (applied after DDC, before demod)
	bpComplex *FIRComplexFilter

	// Squelch
	squelchLevel float64
	squelchOpen  bool
	signalLevel  float64

	// Audio resampler
	audioResampler  *Resampler
	audioResamplerR *Resampler

	// ADS-B
	adsbDecoder  *adsb.Decoder
	adsbTracker  *adsb.Tracker
	aircraftSubs map[chan []adsb.Aircraft]struct{}
	rxLat        float64
	rxLon        float64

	// NOAA APT
	aptDecoder *noaa.APTDecoder
	aptSubs    map[chan noaa.APTLine]struct{}

	// APT lines pending broadcast (collected inside lock, sent outside)
	aptLinesPending []noaa.APTLine

	// Previous sample rate (saved when switching to ADS-B, restored when switching away)
	prevSampleRate uint32

	// Output subscribers (per-client channels)
	audioSubs map[chan AudioBlock]struct{}
	fftSubs   map[chan []float32]struct{}
	statSubs  map[chan Status]struct{}
	subMu     sync.Mutex

	// FFT rate limiting
	fftRate     float64   // target FFT rate in fps
	fftLastTime time.Time // last FFT output time
	running     bool
	stopCh      chan struct{}
	loopDone    chan struct{} // closed when processLoop exits (for source swaps)

	// Configuration
	config ReceiverConfig
}

// AudioBlock holds a block of audio samples (stereo).
type AudioBlock struct {
	Left  []float32
	Right []float32 // nil for mono
}

// Status holds receiver status information.
type Status struct {
	CenterFreq   uint32
	SampleRate   uint32
	SignalLevel  float64 // dBFS
	SquelchOpen  bool
	Demod        string
	FilterLow    float64
	FilterHigh   float64
	FilterOffset float64
	CWOffset     float64
	FilterShape  string

	// Settings (for frontend state recovery after page refresh)
	SquelchLevel   float64 // dBFS, -150 = open
	SpectrumAvg    float64 // FFT averaging factor (0..1)
	FFTRate        float64 // target FFT rate in fps
	FFTMaxHold     bool    // max-hold plot mode
	FFTSize        int     // FFT size
	AutoGain       bool    // SDR auto gain
	Gain           int     // manual gain in tenths of dB
	FreqCorrection int     // ppm
	AGCOn          bool    // AGC enabled
	AGCPreset      string  // AGC preset name

	// Receiver position (for ADS-B CPR decoding)
	RxLat float64
	RxLon float64
}

// NewReceiver creates a new receiver with the given source and config.
func NewReceiver(source SDRDevice, config ReceiverConfig) *Receiver {
	r := &Receiver{
		source:       source,
		spectrum:     NewSpectrumFFT(8192, 0.3),
		ddc:          NewDDC(float64(config.SampleRate), config.FilterOffset, TargetQuadRate),
		agc:          NewAGC(TargetQuadRate),
		agcR:         NewAGC(TargetQuadRate),
		config:       config,
		squelchLevel: config.SquelchLevel,
		filterLow:    config.FilterLow,
		filterHigh:   config.FilterHigh,
		filterOffset: config.FilterOffset,
		filterShape:  FilterShapeNormal,
		cwOffset:     config.CWOffset,
		audioSubs:    make(map[chan AudioBlock]struct{}),
		fftSubs:      make(map[chan []float32]struct{}),
		statSubs:     make(map[chan Status]struct{}),
		aircraftSubs: make(map[chan []adsb.Aircraft]struct{}),
		aptSubs:      make(map[chan noaa.APTLine]struct{}),
		stopCh:       make(chan struct{}),
		loopDone:     func() chan struct{} { c := make(chan struct{}); close(c); return c }(),
		fftRate:      25.0, // 25 fps (gqrx default)
	}

	// Set up audio resamplers
	quadRate := r.ddc.QuadRate()
	r.audioResampler = NewResampler(quadRate, AudioRate)
	r.audioResamplerR = NewResampler(quadRate, AudioRate)

	// Set up AGC (both channels) with medium preset (gqrx default)
	r.agc.SetPreset(AGCPresetMedium)
	r.agcR.SetPreset(AGCPresetMedium)
	r.config.AGCOn = true

	// Set up ADS-B decoder and tracker
	r.adsbDecoder = adsb.NewDecoder(float64(config.SampleRate))
	r.adsbTracker = adsb.NewTracker()

	// Set up NOAA APT decoder (audio rate = 48 kHz)
	r.aptDecoder = noaa.NewAPTDecoder(float64(AudioRate))

	// Set up demodulator (this will set the correct DDC frequency for the mode)
	r.setDemodulator(config.Demod)

	// Set up bandpass filter
	r.updateFilter()

	return r
}

// SubscribeAudio creates a per-client audio channel.
// Buffer of 16 blocks (~500ms at 48kHz) absorbs timing jitter so blocks
// are not dropped when the WebSocket write is momentarily slow.
func (r *Receiver) SubscribeAudio() chan AudioBlock {
	ch := make(chan AudioBlock, 16)
	r.subMu.Lock()
	r.audioSubs[ch] = struct{}{}
	r.subMu.Unlock()
	return ch
}

// UnsubscribeAudio removes a subscriber and closes its channel.
func (r *Receiver) UnsubscribeAudio(ch chan AudioBlock) {
	r.subMu.Lock()
	delete(r.audioSubs, ch)
	r.subMu.Unlock()
	close(ch)
}

// SubscribeFFT creates a per-client FFT channel.
func (r *Receiver) SubscribeFFT() chan []float32 {
	ch := make(chan []float32, 2)
	r.subMu.Lock()
	r.fftSubs[ch] = struct{}{}
	r.subMu.Unlock()
	return ch
}

// UnsubscribeFFT removes a subscriber and closes its channel.
func (r *Receiver) UnsubscribeFFT(ch chan []float32) {
	r.subMu.Lock()
	delete(r.fftSubs, ch)
	r.subMu.Unlock()
	close(ch)
}

// SubscribeStatus creates a per-client status channel.
func (r *Receiver) SubscribeStatus() chan Status {
	ch := make(chan Status, 1)
	r.subMu.Lock()
	r.statSubs[ch] = struct{}{}
	r.subMu.Unlock()
	return ch
}

// UnsubscribeStatus removes a subscriber and closes its channel.
func (r *Receiver) UnsubscribeStatus(ch chan Status) {
	r.subMu.Lock()
	delete(r.statSubs, ch)
	r.subMu.Unlock()
	close(ch)
}

// SubscribeAircraft creates a per-client aircraft data channel.
func (r *Receiver) SubscribeAircraft() chan []adsb.Aircraft {
	ch := make(chan []adsb.Aircraft, 1)
	r.subMu.Lock()
	r.aircraftSubs[ch] = struct{}{}
	r.subMu.Unlock()
	return ch
}

// UnsubscribeAircraft removes a subscriber and closes its channel.
func (r *Receiver) UnsubscribeAircraft(ch chan []adsb.Aircraft) {
	r.subMu.Lock()
	delete(r.aircraftSubs, ch)
	r.subMu.Unlock()
	close(ch)
}

// SetReceiverPosition sets the receiver position for ADS-B CPR decoding.
func (r *Receiver) SetReceiverPosition(lat, lon float64) {
	r.mu.Lock()
	r.rxLat = lat
	r.rxLon = lon
	r.mu.Unlock()
	r.adsbTracker.SetReceiverPosition(lat, lon)
}

// SubscribeAPT creates a per-client APT image line channel.
func (r *Receiver) SubscribeAPT() chan noaa.APTLine {
	ch := make(chan noaa.APTLine, 64)
	r.subMu.Lock()
	r.aptSubs[ch] = struct{}{}
	r.subMu.Unlock()
	return ch
}

// UnsubscribeAPT removes a subscriber and closes its channel.
func (r *Receiver) UnsubscribeAPT(ch chan noaa.APTLine) {
	r.subMu.Lock()
	delete(r.aptSubs, ch)
	r.subMu.Unlock()
	close(ch)
}

// broadcastAPTLine sends an APT line to all subscribers (non-blocking).
func (r *Receiver) broadcastAPTLine(line noaa.APTLine) {
	r.subMu.Lock()
	for ch := range r.aptSubs {
		select {
		case ch <- line:
		default:
		}
	}
	r.subMu.Unlock()
}

// broadcastAudio sends an audio block to all subscribers (non-blocking).
func (r *Receiver) broadcastAudio(block AudioBlock) {
	r.subMu.Lock()
	for ch := range r.audioSubs {
		select {
		case ch <- block:
		default:
		}
	}
	r.subMu.Unlock()
}

// broadcastFFT sends FFT data to all subscribers (non-blocking).
func (r *Receiver) broadcastFFT(data []float32) {
	r.subMu.Lock()
	for ch := range r.fftSubs {
		select {
		case ch <- data:
		default:
		}
	}
	r.subMu.Unlock()
}

// broadcastStatus sends a status update to all subscribers (non-blocking).
func (r *Receiver) broadcastStatus(status Status) {
	r.subMu.Lock()
	for ch := range r.statSubs {
		select {
		case ch <- status:
		default:
		}
	}
	r.subMu.Unlock()
}

// broadcastAircraft sends aircraft data to all subscribers (non-blocking).
func (r *Receiver) broadcastAircraft(aircraft []adsb.Aircraft) {
	r.subMu.Lock()
	for ch := range r.aircraftSubs {
		select {
		case ch <- aircraft:
		default:
		}
	}
	r.subMu.Unlock()
}

// Start starts the receiver processing loop.
// This should be called after the source has started.
func (r *Receiver) Start() {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.stopCh = make(chan struct{})
	r.loopDone = make(chan struct{})
	done := r.loopDone
	r.mu.Unlock()

	go r.processLoop(done)
}

// Stop stops the receiver processing loop.
func (r *Receiver) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return
	}
	close(r.stopCh)
	r.running = false
}

// ReplaceSource atomically swaps the SDR source. It stops the processing
// loop, stops the old source's stream, rebuilds all rate-dependent DSP for
// the new source's sample rate, starts the new source's stream, and restarts
// processing. Safe to call from an HTTP handler while the receiver is running.
func (r *Receiver) ReplaceSource(newSource SDRDevice) {
	// 1. Stop the processing loop and wait for it to exit (without holding
	//    r.mu during the wait, since processLoop needs it to finish its
	//    current block).
	r.mu.Lock()
	wasRunning := r.running
	old := r.source
	if wasRunning {
		close(r.stopCh)
		r.running = false
	}
	done := r.loopDone
	r.mu.Unlock()
	if wasRunning {
		<-done
	}

	// 2. Stop the old source's stream (no one is reading its samples now).
	if old != nil && old != newSource {
		old.Stop()
	}

	// 3. Swap source and rebuild rate-dependent DSP for the new sample rate.
	r.mu.Lock()
	r.source = newSource
	newRate := newSource.GetSampleRate()
	r.config.SampleRate = newRate
	r.config.CenterFreq = newSource.GetCenterFreq()
	r.ddc = NewDDC(float64(newRate), r.filterOffset, TargetQuadRate)
	quadRate := r.ddc.QuadRate()
	r.audioResampler = NewResampler(quadRate, AudioRate)
	r.audioResamplerR = NewResampler(quadRate, AudioRate)
	r.agc.SetSampleRate(quadRate)
	r.agcR.SetSampleRate(quadRate)
	r.adsbDecoder = adsb.NewDecoder(float64(newRate))
	r.aptDecoder = noaa.NewAPTDecoder(float64(AudioRate))
	r.setDemodulator(r.demodType)
	r.mu.Unlock()

	// 4. Start the new source's stream (Start blocks, so run it in a goroutine).
	go func() {
		if err := newSource.Start(); err != nil {
			log.Printf("SDR source error: %v", err)
		}
	}()

	// 5. Restart the processing loop.
	r.Start()
}

// processLoop is the main DSP processing loop.
func (r *Receiver) processLoop(done chan struct{}) {
	defer close(done)
	sampleCh := r.source.Samples()
	statusTicker := 0

	for {
		select {
		case <-r.stopCh:
			return
		case samples, ok := <-sampleCh:
			if !ok {
				return
			}
			r.processBlock(samples)

			// Send periodic status updates
			statusTicker++
			if statusTicker >= 10 {
				statusTicker = 0
				r.sendStatus()
				// Also broadcast aircraft data if in ADS-B mode
				r.mu.Lock()
				if r.demodType == DemodADSB {
					r.adsbTracker.Cleanup()
					aircraft := r.adsbTracker.GetAircraft()
					r.mu.Unlock()
					r.broadcastAircraft(aircraft)
				} else {
					r.mu.Unlock()
				}
			}
		}
	}
}

// processBlock processes a block of IQ samples through the DSP chain.
func (r *Receiver) processBlock(samples []complex128) {
	r.mu.Lock()

	// 1. Compute spectrum (FFT) — rate limited, collect data for broadcast after unlock
	var fftData []float32
	now := time.Now()
	if r.fftRate <= 0 || now.Sub(r.fftLastTime) >= time.Duration(float64(time.Second)/r.fftRate) {
		fftData = r.spectrum.Compute(samples)
		r.fftLastTime = now
	}

	// 2. DDC: frequency shift + decimation
	decimated := r.ddc.Process(samples)

	// 3. Apply bandpass filter
	filtered := r.applyFilter(decimated)

	// 4. Signal level measurement (after filter)
	r.measureSignalLevel(filtered)

	// 5. Squelch check
	r.checkSquelch()

	// 6. Process: ADS-B or audio demodulation
	var audioBlock AudioBlock

	if r.demodType == DemodADSB {
		// Feed raw samples to ADS-B decoder (no DDC needed, signal is at baseband)
		msgs := r.adsbDecoder.Process(samples)
		for _, msg := range msgs {
			r.adsbTracker.ProcessMessage(msg)
		}
	} else if r.demod != nil {
		left, right := r.demod.Process(filtered)

		// 7. AGC (independent per channel so stereo L/R gains don't cross-couple)
		left = r.agc.Process(left)
		if right != nil {
			right = r.agcR.Process(right)
		}

		// 8. Audio resampling
		leftResampled := r.audioResampler.Process(left)
		var rightResampled []float64
		if right != nil {
			rightResampled = r.audioResamplerR.Process(right)
		}

		// Feed to APT decoder if in NOAA mode
		if r.demodType == DemodNOAA && r.aptDecoder != nil {
			aptLines := r.aptDecoder.Process(leftResampled)
			// Queue APT lines for broadcast (outside lock)
			r.aptLinesPending = append(r.aptLinesPending, aptLines...)
		}

		// 9. Convert to float32
		leftF32 := ConvertToFloat32(leftResampled)
		var rightF32 []float32
		if rightResampled != nil {
			rightF32 = ConvertToFloat32(rightResampled)
		}

		// Apply squelch
		if !r.squelchOpen {
			for i := range leftF32 {
				leftF32[i] = 0
			}
			for i := range rightF32 {
				rightF32[i] = 0
			}
		}

		audioBlock = AudioBlock{Left: leftF32, Right: rightF32}
	}

	r.mu.Unlock()

	// Broadcast outside the lock to avoid blocking the DSP pipeline
	if fftData != nil {
		r.broadcastFFT(fftData)
	}
	if audioBlock.Left != nil {
		r.broadcastAudio(audioBlock)
	}
	// Broadcast APT lines if any
	if len(r.aptLinesPending) > 0 {
		for _, line := range r.aptLinesPending {
			r.broadcastAPTLine(line)
		}
		r.aptLinesPending = r.aptLinesPending[:0]
	}
}

// measureSignalLevel computes the RMS signal level in dBFS.
func (r *Receiver) measureSignalLevel(samples []complex128) {
	if len(samples) == 0 {
		return
	}
	var sum float64
	for _, s := range samples {
		mag := real(s)*real(s) + imag(s)*imag(s)
		sum += mag
	}
	rms := math.Sqrt(sum / float64(len(samples)))
	if rms > 0 {
		r.signalLevel = 20 * math.Log10(rms)
	}
}

// checkSquelch checks if the signal is above the squelch threshold.
func (r *Receiver) checkSquelch() {
	if r.squelchLevel <= -150 {
		r.squelchOpen = true
		return
	}
	r.squelchOpen = r.signalLevel >= r.squelchLevel
}

// applyFilter applies the bandpass filter to the samples.
func (r *Receiver) applyFilter(in []complex128) []complex128 {
	if r.bpComplex != nil {
		return r.bpComplex.FilterSlice(in)
	}
	return in
}

// updateFilter updates the bandpass filter based on current filter settings.
func (r *Receiver) updateFilter() {
	sampleRate := r.ddc.QuadRate()
	low := r.filterLow
	high := r.filterHigh

	// Clamp to Nyquist
	nyq := sampleRate / 2
	if high > nyq {
		high = nyq
	}
	if low < -nyq {
		low = -nyq
	}
	if low >= high {
		low = -nyq * 0.1
		high = nyq * 0.1
	}

	// Design the bandpass filter.
	// Number of taps depends on filter shape (more taps = sharper)
	numTaps := 65 // NORMAL default
	switch r.filterShape {
	case FilterShapeSoft:
		numTaps = 33
	case FilterShapeSharp:
		numTaps = 127
	}
	if low < 0 && high > 0 {
		// Passband includes DC (double-sideband modes: AM/NFM/WFM/CW/Raw).
		// A real lowpass is sufficient and has the correct symmetric response.
		taps := DesignLowpass(sampleRate, high, numTaps)
		ctaps := make([]complex128, len(taps))
		for i, t := range taps {
			ctaps[i] = complex(t, 0)
		}
		r.bpComplex = NewFIRComplexFilter(ctaps)
	} else {
		// Asymmetric passband (SSB: USB = [low, high] > 0, LSB = < 0). A real
		// FIR has |H(f)| = |H(-f)| and cannot reject the opposite sideband, so
		// use a complex bandpass (frequency-shifted lowpass) whose response is
		// asymmetric and passes only [center-halfBW, center+halfBW].
		center := (low + high) / 2
		halfBW := high - low
		if halfBW < 0 {
			halfBW = -halfBW
		}
		ctaps := DesignComplexBandpass(sampleRate, center, halfBW, numTaps)
		r.bpComplex = NewFIRComplexFilter(ctaps)
	}
}

// ADS-B recommended sample rate (2 MHz = exactly 2 samples per bit)
const ADSBSampleRate = 2000000

// setDemodulator creates the appropriate demodulator for the given type
// and auto-adjusts the filter bandwidth to the NORMAL preset for that mode
// (matching gqrx behavior exactly).
func (r *Receiver) setDemodulator(dt DemodType) {
	// If switching from ADS-B to another mode, restore the previous sample rate
	if r.demodType == DemodADSB && dt != DemodADSB && r.prevSampleRate != 0 {
		_ = r.reconfigureSampleRate(r.prevSampleRate)
		r.prevSampleRate = 0
	}

	// If switching to ADS-B, ensure sample rate is 2 MHz for reliable decoding
	if dt == DemodADSB && r.config.SampleRate != ADSBSampleRate {
		r.prevSampleRate = r.config.SampleRate
		_ = r.reconfigureSampleRate(ADSBSampleRate)
	}

	r.demodType = dt
	quadRate := r.ddc.QuadRate()

	// Determine if this is a CW mode — CW offset only applies to CW modes.
	isCW := dt == DemodCWL || dt == DemodCWU

	switch dt {
	case DemodNFM:
		r.demod = demod.NewFMDemod(quadRate, 5000, 75e-6)
	case DemodWFM:
		r.demod = demod.NewWFMDemod(quadRate, 75000, false, false)
	case DemodWFMStereo:
		r.demod = demod.NewWFMDemod(quadRate, 75000, true, false)
	case DemodWFMOirt:
		r.demod = demod.NewWFMDemod(quadRate, 75000, true, true)
	case DemodAM:
		r.demod = demod.NewAMDemod(quadRate, true)
	case DemodAMSync:
		r.demod = demod.NewAMSyncDemod(quadRate, true, 0.001)
	case DemodLSB, DemodUSB, DemodCWL, DemodCWU:
		// SSB/CW demodulation: take the real part. Sideband selection
		// is done by the bandpass filter (asymmetric for LSB/USB).
		r.demod = demod.NewSSBDemod(quadRate)
	case DemodRaw:
		// Raw I/Q: pass through without demodulation
		r.demod = demod.NewSSBDemod(quadRate) // real part = I channel
	case DemodADSB:
		// ADS-B: no audio demodulator, raw samples go to ADS-B decoder
		r.demod = nil
	case DemodNOAA:
		// NOAA APT: FM demod with 17 kHz deviation, no de-emphasis
		r.demod = demod.NewFMDemod(quadRate, 17000, 0)
	case DemodOff:
		r.demod = nil
	}

	// Update DDC center frequency: CW offset only applies to CW modes.
	if isCW {
		r.ddc.SetCenterFreq(r.filterOffset - r.cwOffset)
	} else {
		r.ddc.SetCenterFreq(r.filterOffset)
	}

	// Apply NORMAL filter preset for this mode (gqrx default behavior)
	if dt != DemodOff {
		preset := filterPresetTable[dt]
		r.filterLow = preset[FilterPresetNormal][0]
		r.filterHigh = preset[FilterPresetNormal][1]
		r.updateFilter()
	}
}

// sendStatus sends a status update to all subscribers.
func (r *Receiver) sendStatus() {
	status := r.GetStatus()
	r.broadcastStatus(status)
}

// GetAircraft returns the current list of tracked aircraft.
func (r *Receiver) GetAircraft() []adsb.Aircraft {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.adsbTracker.GetAircraft()
}

// GetAircraftHistory returns all aircraft ever tracked (including those
// no longer active), sorted by LastSeen descending.
func (r *Receiver) GetAircraftHistory() []adsb.Aircraft {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.adsbTracker.GetHistory()
}

// GetAllAircraft returns both active and historical aircraft.
func (r *Receiver) GetAllAircraft() []adsb.Aircraft {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.adsbTracker.GetAllAircraft()
}

// --- Control methods ---

// SetCenterFreq sets the center frequency.
func (r *Receiver) SetCenterFreq(freq uint32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config.CenterFreq = freq
	return r.source.SetCenterFreq(freq)
}

// SetFilterOffset sets the filter offset (tuning within passband).
// The DDC center frequency is filterOffset - cwOffset only in CW modes (matching gqrx).
func (r *Receiver) SetFilterOffset(offset float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.filterOffset = offset
	r.updateDDCFreq()
}

// SetCWOffset sets the CW/BFO offset in Hz (gqrx default: 700).
func (r *Receiver) SetCWOffset(offset float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cwOffset = offset
	r.config.CWOffset = offset
	r.updateDDCFreq()
}

// updateDDCFreq updates the DDC center frequency based on current mode.
// CW offset only applies to CW-L/CW-U modes (matching gqrx behavior).
func (r *Receiver) updateDDCFreq() {
	isCW := r.demodType == DemodCWL || r.demodType == DemodCWU
	if isCW {
		r.ddc.SetCenterFreq(r.filterOffset - r.cwOffset)
	} else {
		r.ddc.SetCenterFreq(r.filterOffset)
	}
}

// SetFilterShape sets the filter shape (SOFT/NORMAL/SHARP).
func (r *Receiver) SetFilterShape(shape int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.filterShape = shape
	r.updateFilter()
}

// SetFilterPreset applies a WIDE/NORMAL/NARROW preset for the current demod mode.
func (r *Receiver) SetFilterPreset(preset int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.demodType == DemodOff {
		return
	}
	p := filterPresetTable[r.demodType]
	r.filterLow = p[preset][0]
	r.filterHigh = p[preset][1]
	r.updateFilter()
}

// SetFilter sets the filter bandwidth.
func (r *Receiver) SetFilter(low, high float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.filterLow = low
	r.filterHigh = high
	r.updateFilter()
}

// reconfigureSampleRate changes the sample rate on the source device and
// reinitializes all rate-dependent DSP components (DDC, resamplers, ADS-B decoder).
// Must be called with r.mu held.
func (r *Receiver) reconfigureSampleRate(newRate uint32) error {
	if r.source.GetSampleRate() == newRate {
		return nil
	}
	if err := r.source.SetSampleRate(newRate); err != nil {
		return err
	}
	r.config.SampleRate = newRate
	// Recreate DDC with new input rate
	r.ddc = NewDDC(float64(newRate), r.filterOffset, TargetQuadRate)
	// Recreate audio resamplers (quad rate may have changed)
	quadRate := r.ddc.QuadRate()
	r.audioResampler = NewResampler(quadRate, AudioRate)
	r.audioResamplerR = NewResampler(quadRate, AudioRate)
	// Keep AGC time constants correct for the new quad rate
	r.agc.SetSampleRate(quadRate)
	r.agcR.SetSampleRate(quadRate)
	// Recreate ADS-B decoder with new sample rate
	r.adsbDecoder = adsb.NewDecoder(float64(newRate))
	// Recreate APT decoder (audio rate unchanged but reset state)
	r.aptDecoder = noaa.NewAPTDecoder(float64(AudioRate))
	// Update bandpass filter for new quad rate
	r.updateFilter()
	return nil
}

// GetADSBStats returns ADS-B decoder statistics.
func (r *Receiver) GetADSBStats() (detected, valid, accepted, aircraftCount int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	detected, valid, accepted = r.adsbDecoder.Stats()
	aircraftCount = r.adsbTracker.Count()
	return
}

// GetAPTStats returns NOAA APT decoder statistics.
func (r *Receiver) GetAPTStats() (linesDecoded, syncFound int, signalLevel float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.aptDecoder != nil {
		return r.aptDecoder.Stats()
	}
	return 0, 0, 0
}

// ResetAPT clears the APT decoder state and image buffer.
func (r *Receiver) ResetAPT() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.aptDecoder != nil {
		r.aptDecoder.Reset()
	}
}

// SetDemod sets the demodulator type.
// When switching to/from ADS-B (which requires a different sample rate),
// the processing loop and source stream are stopped first to avoid a
// deadlock: reconfigureSampleRate calls source.SetSampleRate which blocks
// if ReadAsync is running, and the ReadAsync callback blocks on sampleCh
// which processLoop can't drain because it's waiting for r.mu held by us.
func (r *Receiver) SetDemod(dt DemodType) {
	// Determine whether a sample-rate change is required.
	r.mu.Lock()
	needsRateChange := false
	if r.demodType == DemodADSB && dt != DemodADSB && r.prevSampleRate != 0 {
		needsRateChange = true // leaving ADS-B, restore previous rate
	}
	if dt == DemodADSB && r.config.SampleRate != ADSBSampleRate {
		needsRateChange = true // entering ADS-B, switch to 2 MHz
	}
	r.mu.Unlock()

	if !needsRateChange {
		// No sample-rate change: just swap the demodulator under lock.
		r.mu.Lock()
		defer r.mu.Unlock()
		r.setDemodulatorNoRateChange(dt)
		return
	}

	// Sample-rate change required: stop loop + source, reconfigure, restart.
	// 1. Stop the processing loop (don't hold r.mu while waiting for loopDone).
	r.mu.Lock()
	wasRunning := r.running
	if wasRunning {
		close(r.stopCh)
		r.running = false
	}
	done := r.loopDone
	r.mu.Unlock()
	if wasRunning {
		<-done
	}

	// 2. Stop the source stream so SetSampleRate won't block on ReadAsync.
	r.source.Stop()

	// 3. Reconfigure sample rate and demodulator under lock.
	r.mu.Lock()
	r.setDemodulator(dt)
	r.mu.Unlock()

	// 4. Restart the source stream (Start blocks, so run in a goroutine).
	go func() {
		if err := r.source.Start(); err != nil {
			log.Printf("SDR source restart error after demod switch: %v", err)
		}
	}()

	// 5. Restart the processing loop.
	r.Start()
}

// setDemodulatorNoRateChange switches the demodulator without touching the
// sample rate. Used when the new mode uses the same sample rate as the current one.
func (r *Receiver) setDemodulatorNoRateChange(dt DemodType) {
	// Clear prevSampleRate if leaving ADS-B without a rate change
	// (e.g., ADS-B was never actually set to a different rate).
	if r.demodType == DemodADSB && dt != DemodADSB {
		r.prevSampleRate = 0
	}
	r.demodType = dt
	quadRate := r.ddc.QuadRate()
	isCW := dt == DemodCWL || dt == DemodCWU

	switch dt {
	case DemodNFM:
		r.demod = demod.NewFMDemod(quadRate, 5000, 75e-6)
	case DemodWFM:
		r.demod = demod.NewWFMDemod(quadRate, 75000, false, false)
	case DemodWFMStereo:
		r.demod = demod.NewWFMDemod(quadRate, 75000, true, false)
	case DemodWFMOirt:
		r.demod = demod.NewWFMDemod(quadRate, 75000, true, true)
	case DemodAM:
		r.demod = demod.NewAMDemod(quadRate, true)
	case DemodAMSync:
		r.demod = demod.NewAMSyncDemod(quadRate, true, 0.001)
	case DemodLSB, DemodUSB, DemodCWL, DemodCWU:
		r.demod = demod.NewSSBDemod(quadRate)
	case DemodRaw:
		r.demod = demod.NewSSBDemod(quadRate)
	case DemodADSB:
		r.demod = nil
	case DemodNOAA:
		r.demod = demod.NewFMDemod(quadRate, 17000, 0)
	case DemodOff:
		r.demod = nil
	}

	if isCW {
		r.ddc.SetCenterFreq(r.filterOffset - r.cwOffset)
	} else {
		r.ddc.SetCenterFreq(r.filterOffset)
	}

	if dt != DemodOff {
		preset := filterPresetTable[dt]
		r.filterLow = preset[FilterPresetNormal][0]
		r.filterHigh = preset[FilterPresetNormal][1]
		r.updateFilter()
	}
}

// SetSquelch sets the squelch level in dBFS.
func (r *Receiver) SetSquelch(level float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.squelchLevel = level
}

// SetAGC enables or disables AGC.
func (r *Receiver) SetAGC(on bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agc.SetEnabled(on)
	r.agcR.SetEnabled(on)
	r.config.AGCOn = on
}

// SetAutoGain enables or disables auto gain on the SDR.
func (r *Receiver) SetAutoGain(auto bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config.AutoGain = auto
	return r.source.SetAutoGain(auto)
}

// SetGain sets the manual gain in tenths of dB.
func (r *Receiver) SetGain(gain int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config.Gain = gain
	return r.source.SetGain(gain)
}

// SetFreqCorrection sets the frequency correction in ppm.
func (r *Receiver) SetFreqCorrection(ppm int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config.FreqCorrection = ppm
	return r.source.SetFreqCorrection(ppm)
}

// SetSpectrumAvg sets the FFT averaging factor.
func (r *Receiver) SetSpectrumAvg(avg float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spectrum.SetAvg(avg)
}

// SetFFTSize sets the FFT size.
func (r *Receiver) SetFFTSize(size int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spectrum = NewSpectrumFFT(size, r.spectrum.avg)
}

// SetFFTRate sets the FFT update rate in fps (0 = unlimited).
func (r *Receiver) SetFFTRate(fps float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fftRate = fps
}

// SetFFTMaxHold enables or disables max-hold plot mode.
func (r *Receiver) SetFFTMaxHold(on bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spectrum.SetMaxHold(on)
}

// SetAGCPreset sets the AGC to one of the standard presets.
func (r *Receiver) SetAGCPreset(preset AGCPreset) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agc.SetPreset(preset)
	r.agcR.SetPreset(preset)
	r.config.AGCOn = preset != AGCPresetOff
}

// GetSpectrumSize returns the FFT size.
func (r *Receiver) GetSpectrumSize() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.spectrum.Size()
}

// Source returns the SDR source currently feeding the receiver.
func (r *Receiver) Source() SDRDevice {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.source
}

// GetConfig returns the current configuration.
func (r *Receiver) GetConfig() ReceiverConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.config
}

// GetStatus returns the current status.
func (r *Receiver) GetStatus() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Status{
		CenterFreq:     r.source.GetCenterFreq(),
		SampleRate:     r.source.GetSampleRate(),
		SignalLevel:    r.signalLevel,
		SquelchOpen:    r.squelchOpen,
		Demod:          r.demodType.String(),
		FilterLow:      r.filterLow,
		FilterHigh:     r.filterHigh,
		FilterOffset:   r.filterOffset,
		CWOffset:       r.cwOffset,
		FilterShape:    filterShapeName(r.filterShape),
		SquelchLevel:   r.squelchLevel,
		SpectrumAvg:    r.spectrum.avg,
		FFTRate:        r.fftRate,
		FFTMaxHold:     r.spectrum.maxHold,
		FFTSize:        r.spectrum.Size(),
		AutoGain:       r.config.AutoGain,
		Gain:           r.config.Gain,
		FreqCorrection: r.config.FreqCorrection,
		AGCOn:          r.config.AGCOn,
		AGCPreset:      r.agc.GetPreset().String(),
		RxLat:          r.rxLat,
		RxLon:          r.rxLon,
	}
}

// filterShapeName returns the name of a filter shape.
func filterShapeName(shape int) string {
	switch shape {
	case FilterShapeSoft:
		return "Soft"
	case FilterShapeSharp:
		return "Sharp"
	default:
		return "Normal"
	}
}
