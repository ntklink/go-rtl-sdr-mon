package sdr

import (
	"math"
)

// FIRFilter is a real-valued FIR filter.
type FIRFilter struct {
	taps   []float64
	buffer []float64
	idx    int
}

// NewFIRFilter creates a new FIR filter with the given taps.
func NewFIRFilter(taps []float64) *FIRFilter {
	return &FIRFilter{
		taps:   taps,
		buffer: make([]float64, len(taps)),
	}
}

// Filter processes a single sample and returns the filtered output.
func (f *FIRFilter) Filter(x float64) float64 {
	f.buffer[f.idx] = x
	f.idx = (f.idx + 1) % len(f.taps)

	var sum float64
	for i, t := range f.taps {
		j := (f.idx - 1 - i + len(f.taps)) % len(f.taps)
		sum += t * f.buffer[j]
	}
	return sum
}

// FilterSlice processes a slice of samples and returns filtered output.
func (f *FIRFilter) FilterSlice(in []float64) []float64 {
	out := make([]float64, len(in))
	for i, x := range in {
		out[i] = f.Filter(x)
	}
	return out
}

// Taps returns the filter taps.
func (f *FIRFilter) Taps() []float64 {
	return f.taps
}

// FIRComplexFilter is a complex-valued FIR filter.
type FIRComplexFilter struct {
	taps   []complex128
	buffer []complex128
	idx    int
}

// NewFIRComplexFilter creates a new complex FIR filter with the given taps.
func NewFIRComplexFilter(taps []complex128) *FIRComplexFilter {
	return &FIRComplexFilter{
		taps:   taps,
		buffer: make([]complex128, len(taps)),
	}
}

// Filter processes a single complex sample.
func (f *FIRComplexFilter) Filter(x complex128) complex128 {
	f.buffer[f.idx] = x
	f.idx = (f.idx + 1) % len(f.taps)

	var sum complex128
	for i, t := range f.taps {
		j := (f.idx - 1 - i + len(f.taps)) % len(f.taps)
		sum += t * f.buffer[j]
	}
	return sum
}

// FilterSlice processes a slice of complex samples.
func (f *FIRComplexFilter) FilterSlice(in []complex128) []complex128 {
	out := make([]complex128, len(in))
	for i, x := range in {
		out[i] = f.Filter(x)
	}
	return out
}

// DesignLowpass designs a windowed-sinc lowpass FIR filter.
// sampleRate is in Hz, cutoff is the -6dB cutoff frequency in Hz,
// numTaps is the number of filter taps (should be odd).
func DesignLowpass(sampleRate, cutoff float64, numTaps int) []float64 {
	if numTaps%2 == 0 {
		numTaps++
	}
	taps := make([]float64, numTaps)
	fc := cutoff / sampleRate
	mid := (numTaps - 1) / 2.0

	var sum float64
	for i := 0; i < numTaps; i++ {
		if i == mid {
			taps[i] = 2 * fc
		} else {
			taps[i] = math.Sin(2*math.Pi*fc*float64(i-mid)) / (math.Pi * float64(i-mid))
		}
		// Hann window
		w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(numTaps-1))
		taps[i] *= w
		sum += taps[i]
	}

	// Normalize
	for i := range taps {
		taps[i] /= sum
	}

	return taps
}

// DesignBandpass designs a windowed-sinc bandpass FIR filter.
// low and high are the cutoff frequencies in Hz.
func DesignBandpass(sampleRate, low, high float64, numTaps int) []float64 {
	if numTaps%2 == 0 {
		numTaps++
	}
	taps := make([]float64, numTaps)
	fl := low / sampleRate
	fh := high / sampleRate
	mid := (numTaps - 1) / 2.0

	var sum float64
	for i := 0; i < numTaps; i++ {
		n := float64(i - mid)
		if i == mid {
			taps[i] = 2 * (fh - fl)
		} else {
			taps[i] = (math.Sin(2*math.Pi*fh*n) - math.Sin(2*math.Pi*fl*n)) / (math.Pi * n)
		}
		// Hann window
		w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(numTaps-1))
		taps[i] *= w
		sum += taps[i]
	}

	// Normalize to unity gain at center
	if sum != 0 {
		for i := range taps {
			taps[i] /= sum
		}
	}

	return taps
}

// DesignComplexBandpass designs a complex bandpass FIR filter centered at centerFreq.
// This is used for frequency shifting + lowpass filtering in one step.
// The filter shifts the signal by -centerFreq and applies a lowpass with the given half-bandwidth.
func DesignComplexBandpass(sampleRate, centerFreq, halfBandwidth float64, numTaps int) []complex128 {
	if numTaps%2 == 0 {
		numTaps++
	}
	// First design a real lowpass
	lp := DesignLowpass(sampleRate, halfBandwidth, numTaps)

	// Then mix with complex oscillator at -centerFreq
	taps := make([]complex128, numTaps)
	mid := (numTaps - 1) / 2.0
	phaseStep := -2 * math.Pi * centerFreq / sampleRate
	for i := 0; i < numTaps; i++ {
		phase := phaseStep * float64(i-mid)
		taps[i] = complex(lp[i]*math.Cos(phase), lp[i]*math.Sin(phase))
	}

	return taps
}

// IIRFilter is a simple first-order IIR filter used for de-emphasis and DC removal.
type IIRFilter struct {
	a    []float64 // feedforward coefficients
	b    []float64 // feedback coefficients (b[0] is typically 1)
	xBuf []float64
	yBuf []float64
	xIdx int
	yIdx int
}

// NewIIRFilter creates a new IIR filter with the given coefficients.
// a = feedforward, b = feedback (y[n] = sum(a*x[n-i]) - sum(b*y[n-j]) for j>0)
func NewIIRFilter(a, b []float64) *IIRFilter {
	return &IIRFilter{
		a:    a,
		b:    b,
		xBuf: make([]float64, len(a)),
		yBuf: make([]float64, len(b)),
	}
}

// Filter processes a single sample.
func (f *IIRFilter) Filter(x float64) float64 {
	f.xBuf[f.xIdx] = x
	f.xIdx = (f.xIdx + 1) % len(f.a)

	y := 0.0
	for i, a := range f.a {
		j := (f.xIdx - 1 - i + len(f.a)) % len(f.a)
		y += a * f.xBuf[j]
	}
	for j, b := range f.b {
		if j == 0 {
			continue
		}
		k := (f.yIdx - j + len(f.b)) % len(f.b)
		y -= b * f.yBuf[k]
	}

	f.yBuf[f.yIdx] = y
	f.yIdx = (f.yIdx + 1) % len(f.b)

	return y
}

// FilterSlice processes a slice of samples.
func (f *IIRFilter) FilterSlice(in []float64) []float64 {
	out := make([]float64, len(in))
	for i, x := range in {
		out[i] = f.Filter(x)
	}
	return out
}

// DesignDeemphasis designs an FM de-emphasis IIR filter.
// tau is the time constant in seconds (e.g., 75e-6 for US, 50e-6 for Europe).
// sampleRate is in Hz.
// Returns (feedforward, feedback) coefficients.
func DesignDeemphasis(sampleRate, tau float64) ([]float64, []float64) {
	// Single-pole low-pass IIR
	// alpha = dt / (RC + dt) where dt = 1/sampleRate, RC = tau
	dt := 1.0 / sampleRate
	alpha := dt / (tau + dt)
	// y[n] = alpha * x[n] + (1-alpha) * y[n-1]
	a := []float64{alpha}
	b := []float64{1.0, -(1 - alpha)}
	return a, b
}

// DesignDCRemoval designs an IIR DC removal filter.
// alpha controls the time constant (0.999 is typical).
// Returns (feedforward, feedback) coefficients.
func DesignDCRemoval(alpha float64) ([]float64, []float64) {
	// y[n] = x[n] - x[n-1] + alpha * y[n-1]
	a := []float64{1.0, -1.0}
	b := []float64{1.0, -alpha}
	return a, b
}
