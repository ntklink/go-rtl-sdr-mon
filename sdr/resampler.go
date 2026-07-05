package sdr

import (
	"math"
)

// Resampler resamples float64 audio from one sample rate to another
// using linear interpolation.
type Resampler struct {
	inputRate  float64
	outputRate float64
	ratio      float64
	phase      float64
	lastSample float64
}

// NewResampler creates a new resampler.
func NewResampler(inputRate, outputRate float64) *Resampler {
	return &Resampler{
		inputRate:  inputRate,
		outputRate: outputRate,
		ratio:      outputRate / inputRate,
	}
}

// Process resamples a block of samples.
func (r *Resampler) Process(in []float64) []float64 {
	if len(in) == 0 {
		return nil
	}

	// Estimate output length
	outLen := int(math.Ceil(float64(len(in)) * r.ratio))
	out := make([]float64, 0, outLen)

	for r.phase < float64(len(in))-1 {
		idx := int(r.phase)
		frac := r.phase - float64(idx)

		// Linear interpolation
		s := r.lastSample*(1-frac) + in[idx]*frac
		if idx == 0 {
			s = r.lastSample*(1-frac) + in[idx]*frac
		} else {
			s = in[idx-1]*(1-frac) + in[idx]*frac
		}

		out = append(out, s)
		r.phase += 1.0 / r.ratio
	}

	// Adjust phase for next block
	r.phase -= float64(len(in))
	r.lastSample = in[len(in)-1]

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
