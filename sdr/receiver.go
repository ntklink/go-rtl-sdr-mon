package sdr

import (
	"math"
	"sync"
	"time"

	"github.com/ntklink/go-rtl-sdr-mon/sdr/demod"
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
	agc      *AGC

	demod     demod.Demodulator
	demodType DemodType

	// Filter
	filterLow    float64
	filterHigh   float64
	filterOffset float64
	filterShape  int     // FilterShapeSoft/Normal/Sharp
	cwOffset     float64 // CW/BFO offset in Hz

	// Bandpass filter (applied after DDC, before demod)
	bpFilter  *FIRFilter
	bpComplex *FIRComplexFilter

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

	// FFT rate limiting
	fftRate     float64   // target FFT rate in fps
	fftLastTime time.Time // last FFT output time
	running     bool
	stopCh      chan struct{}

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
}

// NewReceiver creates a new receiver with the given source and config.
func NewReceiver(source SDRDevice, config ReceiverConfig) *Receiver {
	r := &Receiver{
		source:       source,
		spectrum:     NewSpectrumFFT(8192, 0.3),
		ddc:          NewDDC(float64(config.SampleRate), config.FilterOffset-config.CWOffset, TargetQuadRate),
		agc:          NewAGC(TargetQuadRate),
		config:       config,
		squelchLevel: config.SquelchLevel,
		filterLow:    config.FilterLow,
		filterHigh:   config.FilterHigh,
		filterOffset: config.FilterOffset,
		filterShape:  FilterShapeNormal,
		cwOffset:     config.CWOffset,
		fftCh:        make(chan []float32, 2),
		audioCh:      make(chan AudioBlock, 4),
		statusCh:     make(chan Status, 1),
		stopCh:       make(chan struct{}),
		fftRate:      25.0, // 25 fps (gqrx default)
	}

	// Set up audio resamplers
	quadRate := r.ddc.QuadRate()
	r.audioResampler = NewResampler(quadRate, AudioRate)
	r.audioResamplerR = NewResampler(quadRate, AudioRate)

	// Set up AGC with medium preset (gqrx default)
	r.agc.SetPreset(AGCPresetMedium)
	r.config.AGCOn = true

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

	// 1. Compute spectrum (FFT) — rate limited
	now := time.Now()
	if r.fftRate <= 0 || now.Sub(r.fftLastTime) >= time.Duration(float64(time.Second)/r.fftRate) {
		fftData := r.spectrum.Compute(samples)
		r.fftLastTime = now
		select {
		case r.fftCh <- fftData:
		default:
			// drop if no consumer
		}
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
	// Number of taps depends on filter shape (more taps = sharper)
	numTaps := 65 // NORMAL default
	switch r.filterShape {
	case FilterShapeSoft:
		numTaps = 33
	case FilterShapeSharp:
		numTaps = 127
	}
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

// setDemodulator creates the appropriate demodulator for the given type
// and auto-adjusts the filter bandwidth to the NORMAL preset for that mode
// (matching gqrx behavior exactly).
func (r *Receiver) setDemodulator(dt DemodType) {
	r.demodType = dt
	quadRate := r.ddc.QuadRate()

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
	case DemodOff:
		r.demod = nil
	}

	// Apply NORMAL filter preset for this mode (gqrx default behavior)
	if dt != DemodOff {
		preset := filterPresetTable[dt]
		r.filterLow = preset[FilterPresetNormal][0]
		r.filterHigh = preset[FilterPresetNormal][1]
		r.updateFilter()
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
		CWOffset:     r.cwOffset,
		FilterShape:  filterShapeName(r.filterShape),
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
// The DDC center frequency is filterOffset - cwOffset (matching gqrx).
func (r *Receiver) SetFilterOffset(offset float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.filterOffset = offset
	r.ddc.SetCenterFreq(offset - r.cwOffset)
}

// SetCWOffset sets the CW/BFO offset in Hz (gqrx default: 700).
func (r *Receiver) SetCWOffset(offset float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cwOffset = offset
	r.config.CWOffset = offset
	r.ddc.SetCenterFreq(r.filterOffset - offset)
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
	r.config.AGCOn = preset != AGCPresetOff
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
		CWOffset:     r.cwOffset,
		FilterShape:  filterShapeName(r.filterShape),
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
