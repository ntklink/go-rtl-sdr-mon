package demod

// SSBDemod implements an SSB demodulator.
// For USB: takes the real part of the IQ signal (after the bandpass filter
// has selected the upper sideband).
// For LSB: takes the imaginary part (or equivalently, the filter selects
// the lower sideband and we take the real part).
// In practice, the sideband selection is done by the receiver's filter offset,
// and the demodulator simply extracts the real component.
type SSBDemod struct {
	quadRate float64
}

// NewSSBDemod creates a new SSB demodulator.
func NewSSBDemod(quadRate float64) *SSBDemod {
	return &SSBDemod{quadRate: quadRate}
}

// Type returns the demodulator type.
// SSBDemod is used for LSB, USB, CW-L, CW-U, and Raw I/Q modes.
// The actual mode is tracked by the receiver; here we return DemodUSB as default.
func (d *SSBDemod) Type() DemodType { return DemodUSB }

// SetQuadRate updates the quadrature sample rate.
func (d *SSBDemod) SetQuadRate(rate float64) { d.quadRate = rate }

// QuadRate returns the current quadrature rate.
func (d *SSBDemod) QuadRate() float64 { return d.quadRate }

// Process demodulates SSB by extracting the real component.
// The sideband selection is done upstream by the filter.
func (d *SSBDemod) Process(in []complex128) (left, right []float64) {
	if len(in) == 0 {
		return nil, nil
	}

	out := make([]float64, len(in))
	for i, s := range in {
		out[i] = real(s)
	}

	return out, nil
}
