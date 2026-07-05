package demod

import (
	"math"
)

// AMDemod implements an AM demodulator using envelope detection
// with optional DC removal.
type AMDemod struct {
	quadRate   float64
	dcrEnabled bool
	dcr        *iirFilter
}

// NewAMDemod creates a new AM demodulator.
// quadRate is the input sample rate in Hz.
// dcr enables DC removal.
func NewAMDemod(quadRate float64, dcr bool) *AMDemod {
	d := &AMDemod{
		quadRate:   quadRate,
		dcrEnabled: dcr,
	}
	if dcr {
		// DC removal IIR: y[n] = x[n] - x[n-1] + 0.999*y[n-1]
		d.dcr = newIIR([]float64{1.0, -1.0}, []float64{1.0, -0.999})
	}
	return d
}

// Type returns the demodulator type.
func (d *AMDemod) Type() DemodType { return DemodAM }

// SetQuadRate updates the quadrature sample rate.
func (d *AMDemod) SetQuadRate(rate float64) { d.quadRate = rate }

// QuadRate returns the current quadrature rate.
func (d *AMDemod) QuadRate() float64 { return d.quadRate }

// SetDCR enables or disables DC removal.
func (d *AMDemod) SetDCR(enabled bool) {
	d.dcrEnabled = enabled
	if enabled && d.dcr == nil {
		d.dcr = newIIR([]float64{1.0, -1.0}, []float64{1.0, -0.999})
	}
}

// Process demodulates AM using envelope detection (magnitude).
func (d *AMDemod) Process(in []complex128) (left, right []float64) {
	if len(in) == 0 {
		return nil, nil
	}

	out := make([]float64, len(in))
	for i, s := range in {
		out[i] = math.Sqrt(real(s)*real(s) + imag(s)*imag(s))
	}

	if d.dcrEnabled && d.dcr != nil {
		out = d.dcr.filterSlice(out)
	}

	return out, nil
}
