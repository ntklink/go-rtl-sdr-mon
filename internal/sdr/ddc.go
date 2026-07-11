package sdr

import (
	"math"
	"math/cmplx"
)

// nco renormalization interval: how many samples the incrementally-rotated
// phasor advances before its magnitude is corrected back to 1. Left
// uncorrected, repeated complex multiplication drifts the magnitude away
// from unity through floating-point rounding.
const ncoRenormInterval = 4096

// DDC is a Digital Down Converter that shifts a desired frequency to baseband
// and decimates the signal to a lower sample rate.
type DDC struct {
	sampleRate float64
	quadRate   float64 // output rate
	decim      int

	// NCO (numerically controlled oscillator), represented as a unit
	// phasor that's incrementally rotated by multiplying with a fixed
	// per-sample step, rather than recomputing cos/sin(phase) from
	// scratch every sample. Valid because the mix frequency is constant
	// between SetCenterFreq calls (unlike a PLL, whose step varies every
	// sample with the error feedback and can't be precomputed this way).
	osc         complex128
	oscStep     complex128
	sinceRenorm int

	// Lowpass filter for decimation
	lpFilter *FIRComplexFilter

	// Decimation counter
	decimCounter int
}

// NewDDC creates a new DDC.
// sampleRate is the input sample rate.
// centerFreq is the offset within the passband to shift to baseband (Hz).
// targetQuadRate is the desired output rate (actual output may differ).
func NewDDC(sampleRate, centerFreq, targetQuadRate float64) *DDC {
	decim := int(sampleRate / targetQuadRate)
	if decim < 1 {
		decim = 1
	}
	quadRate := sampleRate / float64(decim)

	// Design lowpass filter with cutoff at quadRate/2
	// Use a reasonable number of taps
	numTaps := 65
	if numTaps > decim*2+1 {
		numTaps = decim*2 + 1
		if numTaps%2 == 0 {
			numTaps++
		}
	}
	cutoff := quadRate / 2 * 0.9 // 90% of Nyquist
	lpTaps := DesignLowpass(sampleRate, cutoff, numTaps)

	// Convert to complex taps
	ctaps := make([]complex128, len(lpTaps))
	for i, t := range lpTaps {
		ctaps[i] = complex(t, 0)
	}

	ddc := &DDC{
		sampleRate: sampleRate,
		quadRate:   quadRate,
		decim:      decim,
		osc:        1, // unit phasor (phase 0)
		lpFilter:   NewFIRComplexFilter(ctaps),
	}

	ddc.setCenterFreq(centerFreq)
	return ddc
}

// setCenterFreq sets the frequency to shift to baseband.
func (d *DDC) setCenterFreq(freq float64) {
	phaseStep := -2 * math.Pi * freq / d.sampleRate
	d.oscStep = complex(math.Cos(phaseStep), math.Sin(phaseStep))
}

// SetCenterFreq updates the center frequency (thread-safe via caller lock).
func (d *DDC) SetCenterFreq(freq float64) {
	d.setCenterFreq(freq)
}

// QuadRate returns the output sample rate.
func (d *DDC) QuadRate() float64 {
	return d.quadRate
}

// Decim returns the decimation factor.
func (d *DDC) Decim() int {
	return d.decim
}

// Process processes a block of complex samples and returns the decimated, shifted output.
func (d *DDC) Process(in []complex128) []complex128 {
	out := make([]complex128, 0, len(in)/d.decim+1)

	for _, s := range in {
		// Mix with NCO: rotate the unit phasor by the fixed per-sample
		// step instead of recomputing cos/sin(phase) from scratch.
		mixed := s * d.osc
		d.osc *= d.oscStep
		d.sinceRenorm++
		if d.sinceRenorm >= ncoRenormInterval {
			d.sinceRenorm = 0
			d.osc /= complex(cmplx.Abs(d.osc), 0)
		}

		// Filter
		filtered := d.lpFilter.Filter(mixed)

		// Decimate
		d.decimCounter++
		if d.decimCounter >= d.decim {
			d.decimCounter = 0
			out = append(out, filtered)
		}
	}

	return out
}
