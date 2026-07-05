package demod

import (
	"math"
)

// AMSyncDemod implements a synchronous AM demodulator using a PLL
// to track the carrier. This provides better performance than envelope
// detection for weak signals or signals with selective fading.
type AMSyncDemod struct {
	quadRate   float64
	dcrEnabled bool
	dcr        *iirFilter
	pllBW      float64

	// PLL state
	phase    float64
	freq     float64 // current PLL frequency (rad/sample)
	pllGain  float64 // PLL loop gain
	pllAlpha float64 // PLL damping factor

	// Low-pass filter for the demodulated signal
	lpFilter *iirFilter
}

// NewAMSyncDemod creates a new synchronous AM demodulator.
// quadRate is the input sample rate in Hz.
// dcr enables DC removal.
// pllBW is the PLL bandwidth (typical: 0.001).
func NewAMSyncDemod(quadRate float64, dcr bool, pllBW float64) *AMSyncDemod {
	d := &AMSyncDemod{
		quadRate:   quadRate,
		dcrEnabled: dcr,
		pllBW:      pllBW,
		phase:      0,
		freq:       0,
		pllGain:    pllBW,
		pllAlpha:   pllBW * 0.1,
	}

	if dcr {
		d.dcr = newIIR([]float64{1.0, -1.0}, []float64{1.0, -0.999})
	}

	// Simple low-pass for demodulated signal
	dt := 1.0 / quadRate
	alpha := dt / (100e-6 + dt) // 100us time constant
	d.lpFilter = newIIR([]float64{alpha}, []float64{1.0, -(1 - alpha)})

	return d
}

// Type returns the demodulator type.
func (d *AMSyncDemod) Type() DemodType { return DemodAMSync }

// SetQuadRate updates the quadrature sample rate.
func (d *AMSyncDemod) SetQuadRate(rate float64) { d.quadRate = rate }

// QuadRate returns the current quadrature rate.
func (d *AMSyncDemod) QuadRate() float64 { return d.quadRate }

// SetDCR enables or disables DC removal.
func (d *AMSyncDemod) SetDCR(enabled bool) {
	d.dcrEnabled = enabled
	if enabled && d.dcr == nil {
		d.dcr = newIIR([]float64{1.0, -1.0}, []float64{1.0, -0.999})
	}
}

// SetPLLBW sets the PLL bandwidth.
func (d *AMSyncDemod) SetPLLBW(bw float64) {
	d.pllBW = bw
	d.pllGain = bw
	d.pllAlpha = bw * 0.1
}

// Process demodulates AM synchronously using a PLL.
func (d *AMSyncDemod) Process(in []complex128) (left, right []float64) {
	if len(in) == 0 {
		return nil, nil
	}

	out := make([]float64, len(in))
	maxFreq := 2 * math.Pi * 5000 / d.quadRate // ±5kHz PLL lock range

	for i, s := range in {
		// PLL: mix signal with local oscillator
		osc := complex(math.Cos(d.phase), math.Sin(d.phase))
		mixed := complex(real(s)*real(osc)+imag(s)*imag(osc),
			imag(s)*real(osc)-real(s)*imag(osc))

		// Error signal: imaginary part of mixed product
		err := imag(mixed)

		// Update PLL
		d.freq += d.pllAlpha * err
		if d.freq > maxFreq {
			d.freq = maxFreq
		} else if d.freq < -maxFreq {
			d.freq = -maxFreq
		}

		d.phase += d.freq + d.pllGain*err
		if d.phase > 2*math.Pi {
			d.phase -= 2 * math.Pi
		} else if d.phase < 0 {
			d.phase += 2 * math.Pi
		}

		// Demodulated signal: real part of mixed product
		out[i] = real(mixed)
	}

	// Low-pass filter
	out = d.lpFilter.filterSlice(out)

	// DC removal
	if d.dcrEnabled && d.dcr != nil {
		out = d.dcr.filterSlice(out)
	}

	return out, nil
}
