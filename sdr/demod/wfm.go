package demod

import (
	"math"
)

// WFMDemod implements a wide-band FM demodulator for broadcast FM.
// It supports mono and stereo demodulation.
// The stereo demodulator extracts the 19kHz pilot tone, regenerates
// the 38kHz subcarrier, and demodulates the L-R DSB-SC signal.
type WFMDemod struct {
	quadRate float64
	maxDev   float64
	gain     float64
	stereo   bool
	oirt     bool // OIRT stereo standard (31.25kHz pilot)

	prevSample complex128

	// De-emphasis (only for stereo, gqrx disables for mono)
	deemphL *iirFilter
	deemphR *iirFilter

	// Pilot PLL
	pilotPhase float64
	pilotFreq  float64 // rad/sample
	pilotGain  float64
	pilotAlpha float64
	pilotHz    float64 // nominal pilot frequency in Hz

	// Audio low-pass
	audioLP *iirFilter
}

// NewWFMDemod creates a new wide-band FM demodulator.
// quadRate is the input sample rate (typically 240kHz).
// maxDev is the maximum deviation (75000 Hz for broadcast FM).
// stereo enables stereo demodulation.
// oirt selects the OIRT stereo standard (31.25kHz pilot) instead of standard (19kHz).
func NewWFMDemod(quadRate, maxDev float64, stereo, oirt bool) *WFMDemod {
	d := &WFMDemod{
		quadRate: quadRate,
		maxDev:   maxDev,
		stereo:   stereo,
		oirt:     oirt,
		gain:     quadRate / (2 * math.Pi * maxDev),
	}

	// Pilot frequency: 19kHz standard, 31.25kHz OIRT
	if oirt {
		d.pilotHz = 31250.0
	} else {
		d.pilotHz = 19000.0
	}

	// De-emphasis (50µs, only applied for stereo — gqrx disables for mono)
	tau := 50e-6
	dt := 1.0 / quadRate
	alpha := dt / (tau + dt)
	d.deemphL = newIIR([]float64{alpha}, []float64{1.0, -(1 - alpha)})
	d.deemphR = newIIR([]float64{alpha}, []float64{1.0, -(1 - alpha)})

	// Pilot PLL parameters
	d.pilotFreq = 2 * math.Pi * d.pilotHz / quadRate
	d.pilotGain = 0.001
	d.pilotAlpha = 0.0001

	// Audio low-pass filter: 17kHz standard, 15kHz OIRT (matches gqrx stereo_demod)
	cutoffHz := 17000.0
	if oirt {
		cutoffHz = 15000.0
	}
	audioAlpha := dt / (1.0/(2*math.Pi*cutoffHz) + dt)
	d.audioLP = newIIR([]float64{audioAlpha}, []float64{1.0, -(1 - audioAlpha)})

	return d
}

// Type returns the demodulator type.
func (d *WFMDemod) Type() DemodType {
	if d.oirt {
		return DemodWFMOirt
	}
	if d.stereo {
		return DemodWFMStereo
	}
	return DemodWFM
}

// SetQuadRate updates the quadrature sample rate.
func (d *WFMDemod) SetQuadRate(rate float64) {
	d.quadRate = rate
	d.gain = rate / (2 * math.Pi * d.maxDev)
	d.pilotFreq = 2 * math.Pi * d.pilotHz / rate
}

// QuadRate returns the current quadrature rate.
func (d *WFMDemod) QuadRate() float64 { return d.quadRate }

// SetStereo enables or disables stereo demodulation.
func (d *WFMDemod) SetStereo(stereo bool) {
	d.stereo = stereo
}

// Process demodulates wide-band FM.
// For mono: returns (left, nil).
// For stereo: returns (left, right).
func (d *WFMDemod) Process(in []complex128) (left, right []float64) {
	if len(in) == 0 {
		return nil, nil
	}

	// FM demodulation (quadrature detection)
	demod := make([]float64, len(in))
	for i, s := range in {
		if i == 0 && d.prevSample == 0 {
			d.prevSample = s
			demod[i] = 0
			continue
		}
		product := cmplxConjMul(d.prevSample, s)
		phase := math.Atan2(imag(product), real(product))
		demod[i] = phase * d.gain
		d.prevSample = s
	}

	if !d.stereo {
		// Mono: low-pass filter + de-emphasis (50µs, matches gqrx wfmrx)
		mono := d.audioLP.filterSlice(demod)
		mono = d.deemphL.filterSlice(mono)
		return mono, nil
	}

	// Stereo demodulation
	// 1. Extract pilot tone (19kHz) using PLL
	// 2. Generate 38kHz subcarrier (2x pilot)
	// 3. Demodulate L-R from 38kHz DSB-SC
	// 4. L+R is the baseband audio (< 15kHz)
	// 5. L = (L+R + L-R) / 2, R = (L+R - L-R) / 2

	lr := make([]float64, len(demod))  // L+R (baseband)
	lmr := make([]float64, len(demod)) // L-R (after stereo demod)

	for i, x := range demod {
		// Pilot PLL
		pilot := math.Cos(d.pilotPhase)
		err := x * pilot // correlate with pilot

		d.pilotFreq += d.pilotAlpha * err
		// Limit PLL frequency range
		nominalFreq := 2 * math.Pi * d.pilotHz / d.quadRate
		maxDev := 2 * math.Pi * 100 / d.quadRate // ±100Hz
		if d.pilotFreq > nominalFreq+maxDev {
			d.pilotFreq = nominalFreq + maxDev
		} else if d.pilotFreq < nominalFreq-maxDev {
			d.pilotFreq = nominalFreq - maxDev
		}

		d.pilotPhase += d.pilotFreq + d.pilotGain*err
		if d.pilotPhase > 2*math.Pi {
			d.pilotPhase -= 2 * math.Pi
		} else if d.pilotPhase < 0 {
			d.pilotPhase += 2 * math.Pi
		}

		// L+R: baseband audio (low-pass the demodulated signal)
		lr[i] = x

		// L-R: demodulate 38kHz DSB-SC
		// Multiply by 2*cos(2*pilotPhase) to shift 38kHz to baseband
		subcarrier := 2 * math.Cos(2*d.pilotPhase)
		lmr[i] = x * subcarrier
	}

	// Low-pass filter both channels (15kHz)
	lr = d.audioLP.filterSlice(lr)
	lmr = d.audioLP.filterSlice(lmr)

	// De-emphasis
	lr = d.deemphL.filterSlice(lr)
	lmr = d.deemphR.filterSlice(lmr)

	// Combine: L = (L+R + L-R) / 2, R = (L+R - L-R) / 2
	leftOut := make([]float64, len(lr))
	rightOut := make([]float64, len(lr))
	for i := range lr {
		leftOut[i] = (lr[i] + lmr[i]) * 0.5
		rightOut[i] = (lr[i] - lmr[i]) * 0.5
	}

	return leftOut, rightOut
}
