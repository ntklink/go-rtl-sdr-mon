package sdr

import (
	"math"
)

// DDC is a Digital Down Converter that shifts a desired frequency to baseband
// and decimates the signal to a lower sample rate.
type DDC struct {
	sampleRate float64
	quadRate   float64 // output rate
	decim      int

	// NCO (numerically controlled oscillator)
	phase     float64
	phaseStep float64

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
		lpFilter:   NewFIRComplexFilter(ctaps),
	}

	ddc.setCenterFreq(centerFreq)
	return ddc
}

// setCenterFreq sets the frequency to shift to baseband.
func (d *DDC) setCenterFreq(freq float64) {
	d.phaseStep = -2 * math.Pi * freq / d.sampleRate
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
		// Mix with NCO
		osc := complex(math.Cos(d.phase), math.Sin(d.phase))
		mixed := s * osc
		d.phase += d.phaseStep
		if d.phase > 2*math.Pi {
			d.phase -= 2 * math.Pi
		} else if d.phase < 0 {
			d.phase += 2 * math.Pi
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
