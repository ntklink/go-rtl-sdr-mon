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

	// Estimate output length
	outLen := int(math.Ceil(float64(len(src)) * r.ratio))
	out := make([]float64, 0, outLen)

	for r.phase < float64(len(src))-1 {
		idx := int(r.phase)
		frac := r.phase - float64(idx)

		// Linear interpolation
		var s float64
		if idx == 0 {
			s = r.lastSample*(1-frac) + src[idx]*frac
		} else {
			s = src[idx-1]*(1-frac) + src[idx]*frac
		}

		out = append(out, s)
		r.phase += 1.0 / r.ratio
	}

	// Adjust phase for next block
	r.phase -= float64(len(src))
	r.lastSample = src[len(src)-1]

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
	for phase < float64(len(in))-1 {
		idx := int(phase)
		frac := phase - float64(idx)

		var s complex128
		if idx == 0 {
			s = in[0]
		} else {
			re := real(in[idx-1])*(1-frac) + real(in[idx])*frac
			im := imag(in[idx-1])*(1-frac) + imag(in[idx])*frac
			s = complex(re, im)
		}

		out = append(out, s)
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
