package sdr

import (
	"math"
)

// FIRFilter is a real-valued FIR filter.
//
// The delay line is a mirrored double-length ring buffer: each incoming
// sample is written to both buffer[pos] and buffer[pos+n], so the most
// recent n samples are always readable as one contiguous, in-order slice
// (buffer[pos:pos+n]) without a per-tap modulo or wraparound check. taps is
// stored reversed (newest-first) at construction so Filter is a plain
// contiguous dot product.
type FIRFilter struct {
	origTaps []float64 // original order, for Taps()
	taps     []float64 // reversed, paired with buffer[pos:pos+n]
	buffer   []float64 // length 2*n, mirrored
	n        int
	pos      int
}

// NewFIRFilter creates a new FIR filter with the given taps.
func NewFIRFilter(taps []float64) *FIRFilter {
	n := len(taps)
	rev := make([]float64, n)
	for i, t := range taps {
		rev[n-1-i] = t
	}
	return &FIRFilter{
		origTaps: taps,
		taps:     rev,
		buffer:   make([]float64, 2*n),
		n:        n,
	}
}

// Filter processes a single sample and returns the filtered output.
func (f *FIRFilter) Filter(x float64) float64 {
	f.buffer[f.pos] = x
	f.buffer[f.pos+f.n] = x
	f.pos++
	if f.pos == f.n {
		f.pos = 0
	}

	window := f.buffer[f.pos : f.pos+f.n]
	var sum float64
	for i, t := range f.taps {
		sum += t * window[i]
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

// Taps returns the filter taps in their original (non-reversed) order.
func (f *FIRFilter) Taps() []float64 {
	return f.origTaps
}

// FIRComplexFilter is a complex-valued FIR filter. See FIRFilter for the
// mirrored-ring-buffer technique used to avoid per-tap modulo indexing.
type FIRComplexFilter struct {
	origTaps []complex128
	taps     []complex128
	buffer   []complex128
	n        int
	pos      int
}

// NewFIRComplexFilter creates a new complex FIR filter with the given taps.
func NewFIRComplexFilter(taps []complex128) *FIRComplexFilter {
	n := len(taps)
	rev := make([]complex128, n)
	for i, t := range taps {
		rev[n-1-i] = t
	}
	return &FIRComplexFilter{
		origTaps: taps,
		taps:     rev,
		buffer:   make([]complex128, 2*n),
		n:        n,
	}
}

// Filter processes a single complex sample.
func (f *FIRComplexFilter) Filter(x complex128) complex128 {
	f.buffer[f.pos] = x
	f.buffer[f.pos+f.n] = x
	f.pos++
	if f.pos == f.n {
		f.pos = 0
	}

	window := f.buffer[f.pos : f.pos+f.n]
	var sum complex128
	for i, t := range f.taps {
		sum += t * window[i]
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

// DesignComplexBandpass designs a complex bandpass FIR filter whose passband
// is [centerFreq-halfBandwidth, centerFreq+halfBandwidth]. It is a lowpass
// frequency-shifted to +centerFreq, so its magnitude response is asymmetric
// (|H(f)| != |H(-f)|) and it can select a single sideband — unlike a real
// bandpass, which is always symmetric.
func DesignComplexBandpass(sampleRate, centerFreq, halfBandwidth float64, numTaps int) []complex128 {
	if numTaps%2 == 0 {
		numTaps++
	}
	// First design a real lowpass (unity DC gain).
	lp := DesignLowpass(sampleRate, halfBandwidth, numTaps)

	// Shift the lowpass to +centerFreq: h[k] = lp[k] * exp(+j*wc*k).
	// This yields H(w) = LP(w - wc), which peaks at w = +wc (i.e. +centerFreq).
	taps := make([]complex128, numTaps)
	mid := (numTaps - 1) / 2.0
	phaseStep := 2 * math.Pi * centerFreq / sampleRate
	for i := 0; i < numTaps; i++ {
		phase := phaseStep * float64(i-mid)
		taps[i] = complex(lp[i]*math.Cos(phase), lp[i]*math.Sin(phase))
	}

	return taps
}
