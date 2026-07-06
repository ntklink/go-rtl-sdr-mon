package sdr

import (
	"math"
)

// Resampler resamples float64 audio from one sample rate to another.
// It applies an anti-aliasing FIR low-pass filter before linear interpolation
// to prevent aliasing when downsampling.
type Resampler struct {
	inputRate  float64
	outputRate float64
	ratio      float64
	phase      float64
	lastSample float64

	// Anti-aliasing filter (used when downsampling)
	aaFilter *FIRFilter
	aaBuf    []float64
}

// NewResampler creates a new resampler.
func NewResampler(inputRate, outputRate float64) *Resampler {
	r := &Resampler{
		inputRate:  inputRate,
		outputRate: outputRate,
		ratio:      outputRate / inputRate,
	}

	// When downsampling, create an anti-aliasing filter
	// with cutoff at outputRate/2
	if outputRate < inputRate {
		numTaps := 63
		cutoff := outputRate / 2 * 0.9 // 90% of output Nyquist
		taps := DesignLowpass(inputRate, cutoff, numTaps)
		r.aaFilter = NewFIRFilter(taps)
		r.aaBuf = make([]float64, numTaps)
	}

	return r
}

// Process resamples a block of samples.
func (r *Resampler) Process(in []float64) []float64 {
	if len(in) == 0 {
		return nil
	}

	// Apply anti-aliasing filter if downsampling
	var src []float64
	if r.aaFilter != nil {
		src = make([]float64, len(in))
		for i, x := range in {
			src[i] = r.aaFilter.Filter(x)
		}
	} else {
		src = in
	}

	n := len(src)

	// Estimate output length
	outLen := int(math.Ceil(float64(n) * r.ratio))
	out := make([]float64, 0, outLen)

	// Linear interpolation over the extended stream [lastSample, src[0..n-1]].
	// r.phase is the fractional position of the next output sample, carried
	// across blocks so the stream stays continuous:
	//   position 0 == lastSample, position k (k>=1) == src[k-1].
	// Interpolating between position idx (E[idx]) and idx+1 (E[idx+1]) requires
	// idx+1 <= n, i.e. r.phase < n.
	for r.phase < float64(n) {
		idx := int(r.phase)
		frac := r.phase - float64(idx)
		a := r.lastSample
		if idx > 0 {
			a = src[idx-1]
		}
		b := src[idx]
		out = append(out, a*(1-frac)+b*frac)
		r.phase += 1.0 / r.ratio
	}

	// Carry the remaining fractional position and the last sample to next block.
	r.phase -= float64(n)
	r.lastSample = src[n-1]

	return out
}

// ResampleComplex resamples complex128 samples using linear interpolation.
func ResampleComplex(in []complex128, inputRate, outputRate float64) []complex128 {
	if len(in) == 0 || inputRate == outputRate {
		return in
	}

	ratio := outputRate / inputRate
	outLen := int(math.Ceil(float64(len(in)) * ratio))
	out := make([]complex128, 0, outLen)

	phase := 0.0
	// Interpolate between in[idx] and in[idx+1]; needs idx+1 <= len(in)-1.
	for phase < float64(len(in))-1 {
		idx := int(phase)
		frac := phase - float64(idx)
		re := real(in[idx])*(1-frac) + real(in[idx+1])*frac
		im := imag(in[idx])*(1-frac) + imag(in[idx+1])*frac
		out = append(out, complex(re, im))
		phase += 1.0 / ratio
	}

	return out
}

// ConvertToFloat32 converts float64 slice to float32.
func ConvertToFloat32(in []float64) []float32 {
	out := make([]float32, len(in))
	for i, v := range in {
		out[i] = float32(v)
	}
	return out
}

// ConvertToInt16 converts float64 samples to int16 PCM.
// Assumes input is in range [-1, 1].
func ConvertToInt16(in []float64) []int16 {
	out := make([]int16, len(in))
	for i, v := range in {
		s := v * 32767
		if s > 32767 {
			s = 32767
		} else if s < -32768 {
			s = -32768
		}
		out[i] = int16(s)
	}
	return out
}
