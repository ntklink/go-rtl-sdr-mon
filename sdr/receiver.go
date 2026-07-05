package sdr

import (
	"math"
	"sync"

	"github.com/ntklink/go-rtl-sdr-mon/sdr/demod"
)

// DemodType re-exports the demod type from the demod package for convenience.
type DemodType = demod.DemodType

// Demod constants (re-exported for API use).
const (
	DemodNone      = demod.DemodNone
	DemodAM        = demod.DemodAM
	DemodNFM       = demod.DemodNFM
	DemodWFM       = demod.DemodWFM
	DemodWFMStereo = demod.DemodWFMStereo
	DemodSSB       = demod.DemodSSB
	DemodAMSync    = demod.DemodAMSync
)

// AudioRate is the standard audio output sample rate.
const AudioRate = 48000

// TargetQuadRate is the target quadrature rate after DDC decimation.
const TargetQuadRate = 250000

// ReceiverConfig holds configuration for the receiver.
type ReceiverConfig struct {
	SampleRate     uint32
	CenterFreq     uint32
	FilterLow      float64 // Hz relative to center
	FilterHigh     float64 // Hz relative to center
	FilterOffset   float64 // Hz, offset from center frequency
	Demod          DemodType
	SquelchLevel   float64 // dBFS, -150 = open
	AGCOn          bool
	Gain           int // tenths of dB
	AutoGain       bool
	FreqCorrection int // ppm
}

// DefaultReceiverConfig returns a default configuration.
func DefaultReceiverConfig() ReceiverConfig {
	return ReceiverConfig{
		SampleRate:     2400000,
		CenterFreq:     100000000, // 100 MHz
		FilterLow:      -5000,
		FilterHigh:     5000,
		FilterOffset:   0,
		Demod:          DemodNFM,
		SquelchLevel:   -150,
		AGCOn:          true,
		AutoGain:       true,
		FreqCorrection: 0,
	}
}

// Receiver is the top-level receiver that orchestrates the DSP chain.
type Receiver struct {
	mu sync.Mutex

	source   *Source
	spectrum *SpectrumFFT
	ddc      *DDC
	agc      *AGC

	demod     demod.Demodulator
	demodType DemodType

	// Bandpass filter (applied after DDC, before demod)
	bpFilter     *FIRFilter
	bpComplex    *FIRComplexFilter
	filterLow    float64
	filterHigh   float64
	filterOffset float64

	// Squelch
	squelchLevel float64
	squelchOpen  bool
	signalLevel  float64

	// Audio resampler
	audioResampler  *Resampler
	audioResamplerR *Resampler

	// Output channels
	fftCh    chan []float32
	audioCh  chan AudioBlock
	statusCh chan Status

	running bool
	stopCh  chan struct{}

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
}

// NewReceiver creates a new receiver with the given source and config.
func NewReceiver(source *Source, config ReceiverConfig) *Receiver {
	r := &Receiver{
		source:       source,
		spectrum:     NewSpectrumFFT(2048, 0.3),
		ddc:          NewDDC(float64(config.SampleRate), config.FilterOffset, TargetQuadRate),
		agc:          NewAGC(TargetQuadRate),
		config:       config,
		squelchLevel: config.SquelchLevel,
		filterLow:    config.FilterLow,
		filterHigh:   config.FilterHigh,
		filterOffset: config.FilterOffset,
		fftCh:        make(chan []float32, 2),
		audioCh:      make(chan AudioBlock, 4),
		statusCh:     make(chan Status, 1),
		stopCh:       make(chan struct{}),
	}

	// Set up audio resamplers
	quadRate := r.ddc.QuadRate()
	r.audioResampler = NewResampler(quadRate, AudioRate)
	r.audioResamplerR = NewResampler(quadRate, AudioRate)

	// Set up AGC
	r.agc.SetEnabled(config.AGCOn)

	// Set up demodulator
	r.setDemodulator(config.Demod)

	// Set up bandpass filter
	r.updateFilter()

	return r
}

// FFTCh returns the channel for FFT spectrum data.
func (r *Receiver) FFTCh() <-chan []float32 { return r.fftCh }

// AudioCh returns the channel for audio data.
func (r *Receiver) AudioCh() <-chan AudioBlock { return r.audioCh }

// StatusCh returns the channel for status updates.
func (r *Receiver) StatusCh() <-chan Status { return r.statusCh }

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
	r.mu.Unlock()

	go r.processLoop()
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

// processLoop is the main DSP processing loop.
func (r *Receiver) processLoop() {
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
			}
		}
	}
}

// processBlock processes a block of IQ samples through the DSP chain.
func (r *Receiver) processBlock(samples []complex128) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 1. Compute spectrum (FFT)
	fftData := r.spectrum.Compute(samples)
	select {
	case r.fftCh <- fftData:
	default:
		// drop if no consumer
	}

	// 2. DDC: frequency shift + decimation
	decimated := r.ddc.Process(samples)

	// 3. Apply bandpass filter
	filtered := r.applyFilter(decimated)

	// 4. Signal level measurement (after filter)
	r.measureSignalLevel(filtered)

	// 5. Squelch check
	r.checkSquelch()

	// 6. Demodulate
	if r.demod != nil {
		left, right := r.demod.Process(filtered)

		// 7. AGC
		left = r.agc.Process(left)
		if right != nil {
			right = r.agc.Process(right)
		}

		// 8. Audio resampling
		leftResampled := r.audioResampler.Process(left)
		var rightResampled []float64
		if right != nil {
			rightResampled = r.audioResamplerR.Process(right)
		}

		// 9. Convert to float32 and send
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

		audioBlock := AudioBlock{Left: leftF32, Right: rightF32}
		select {
		case r.audioCh <- audioBlock:
		default:
			// drop if no consumer
		}
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

	// Design complex bandpass filter centered at 0
	// Actually, we use a real bandpass since the signal is already at baseband
	// after DDC. We need a bandpass that passes [low, high].
	numTaps := 65
	if low < 0 && high > 0 {
		// Bandpass includes DC, so use a lowpass with cutoff = high
		taps := DesignLowpass(sampleRate, high, numTaps)
		ctaps := make([]complex128, len(taps))
		for i, t := range taps {
			ctaps[i] = complex(t, 0)
		}
		r.bpComplex = NewFIRComplexFilter(ctaps)
	} else {
		// True bandpass
		taps := DesignBandpass(sampleRate, math.Abs(low), math.Abs(high), numTaps)
		ctaps := make([]complex128, len(taps))
		for i, t := range taps {
			ctaps[i] = complex(t, 0)
		}
		r.bpComplex = NewFIRComplexFilter(ctaps)
	}
}

// setDemodulator creates the appropriate demodulator for the given type.
func (r *Receiver) setDemodulator(dt DemodType) {
	r.demodType = dt
	quadRate := r.ddc.QuadRate()

	switch dt {
	case DemodNFM:
		r.demod = demod.NewFMDemod(quadRate, 5000, 75e-6)
	case DemodWFM:
		r.demod = demod.NewWFMDemod(quadRate, 75000, false)
	case DemodWFMStereo:
		r.demod = demod.NewWFMDemod(quadRate, 75000, true)
	case DemodAM:
		r.demod = demod.NewAMDemod(quadRate, true)
	case DemodAMSync:
		r.demod = demod.NewAMSyncDemod(quadRate, true, 0.001)
	case DemodSSB:
		r.demod = demod.NewSSBDemod(quadRate)
	case DemodNone:
		r.demod = nil
	}
}

// sendStatus sends a status update.
func (r *Receiver) sendStatus() {
	r.mu.Lock()
	status := Status{
		CenterFreq:   r.source.GetCenterFreq(),
		SampleRate:   r.source.GetSampleRate(),
		SignalLevel:  r.signalLevel,
		SquelchOpen:  r.squelchOpen,
		Demod:        r.demodType.String(),
		FilterLow:    r.filterLow,
		FilterHigh:   r.filterHigh,
		FilterOffset: r.filterOffset,
	}
	r.mu.Unlock()

	select {
	case r.statusCh <- status:
	default:
	}
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
func (r *Receiver) SetFilterOffset(offset float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.filterOffset = offset
	r.ddc.SetCenterFreq(offset)
}

// SetFilter sets the filter bandwidth.
func (r *Receiver) SetFilter(low, high float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.filterLow = low
	r.filterHigh = high
	r.updateFilter()
}

// SetDemod sets the demodulator type.
func (r *Receiver) SetDemod(dt DemodType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setDemodulator(dt)
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

// GetSpectrumSize returns the FFT size.
func (r *Receiver) GetSpectrumSize() int {
	return r.spectrum.Size()
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
		CenterFreq:   r.source.GetCenterFreq(),
		SampleRate:   r.source.GetSampleRate(),
		SignalLevel:  r.signalLevel,
		SquelchOpen:  r.squelchOpen,
		Demod:        r.demodType.String(),
		FilterLow:    r.filterLow,
		FilterHigh:   r.filterHigh,
		FilterOffset: r.filterOffset,
	}
}
