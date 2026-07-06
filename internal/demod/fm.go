package demod

import (
	"math"
)

// FMDemod implements an FM demodulator using quadrature detection.
// It supports both narrow-band and wide-band FM with de-emphasis.
type FMDemod struct {
	quadRate    float64
	maxDev      float64 // maximum deviation in Hz
	tau         float64 // de-emphasis time constant (0 = disabled)
	gain        float64 // demodulator gain
	prevSample  complex128
	initialized bool // whether prevSample holds a real sample
	deemph      *iirFilter
	hasDeemph   bool
}

// iirFilter is a simple first-order IIR filter (local copy to avoid import cycle).
type iirFilter struct {
	a    []float64
	b    []float64
	xBuf []float64
	yBuf []float64
	xIdx int
	yIdx int
}

func newIIR(a, b []float64) *iirFilter {
	return &iirFilter{
		a:    a,
		b:    b,
		xBuf: make([]float64, len(a)),
		yBuf: make([]float64, len(b)),
	}
}

func (f *iirFilter) filter(x float64) float64 {
	f.xBuf[f.xIdx] = x
	f.xIdx = (f.xIdx + 1) % len(f.a)

	y := 0.0
	for i, a := range f.a {
		j := (f.xIdx - 1 - i + len(f.a)) % len(f.a)
		y += a * f.xBuf[j]
	}
	for j, b := range f.b {
		if j == 0 {
			continue
		}
		k := (f.yIdx - j + len(f.b)) % len(f.b)
		y -= b * f.yBuf[k]
	}

	f.yBuf[f.yIdx] = y
	f.yIdx = (f.yIdx + 1) % len(f.b)

	return y
}

func (f *iirFilter) filterSlice(in []float64) []float64 {
	out := make([]float64, len(in))
	for i, x := range in {
		out[i] = f.filter(x)
	}
	return out
}

// NewFMDemod creates a new FM demodulator.
// quadRate is the input sample rate in Hz.
// maxDev is the maximum FM deviation in Hz (5000 for NFM, 75000 for WFM).
// tau is the de-emphasis time constant (75e-6 for US, 50e-6 for Europe, 0 to disable).
func NewFMDemod(quadRate, maxDev, tau float64) *FMDemod {
	d := &FMDemod{
		quadRate: quadRate,
		maxDev:   maxDev,
		tau:      tau,
		gain:     quadRate / (2 * math.Pi * maxDev),
	}
	d.updateDeemph()
	return d
}

func (d *FMDemod) updateDeemph() {
	if d.tau > 0 {
		dt := 1.0 / d.quadRate
		alpha := dt / (d.tau + dt)
		d.deemph = newIIR([]float64{alpha}, []float64{1.0, -(1 - alpha)})
		d.hasDeemph = true
	} else {
		d.hasDeemph = false
	}
}

// Type returns the demodulator type.
func (d *FMDemod) Type() DemodType { return DemodNFM }

// SetQuadRate updates the quadrature sample rate and recalculates gain.
func (d *FMDemod) SetQuadRate(rate float64) {
	d.quadRate = rate
	d.gain = rate / (2 * math.Pi * d.maxDev)
	d.updateDeemph()
}

// QuadRate returns the current quadrature rate.
func (d *FMDemod) QuadRate() float64 { return d.quadRate }

// SetMaxDev sets the maximum FM deviation.
func (d *FMDemod) SetMaxDev(maxDev float64) {
	d.maxDev = maxDev
	d.gain = d.quadRate / (2 * math.Pi * maxDev)
}

// SetDeemph sets the de-emphasis time constant (0 to disable).
func (d *FMDemod) SetDeemph(tau float64) {
	d.tau = tau
	d.updateDeemph()
}

// Process demodulates FM using quadrature detection.
func (d *FMDemod) Process(in []complex128) (left, right []float64) {
	if len(in) == 0 {
		return nil, nil
	}

	out := make([]float64, len(in))

	for i, s := range in {
		if i == 0 && !d.initialized {
			// First sample ever: no previous sample to differentiate against.
			d.prevSample = s
			d.initialized = true
			out[i] = 0
			continue
		}

		// Quadrature demod: arg(conj(prev) * current) * gain
		product := cmplxConjMul(d.prevSample, s)
		phase := math.Atan2(imag(product), real(product))
		out[i] = phase * d.gain

		d.prevSample = s
	}

	// Apply de-emphasis
	if d.hasDeemph {
		out = d.deemph.filterSlice(out)
	}

	return out, nil
}

// cmplxConjMul computes conj(a) * b.
func cmplxConjMul(a, b complex128) complex128 {
	ar, ai := real(a), imag(a)
	br, bi := real(b), imag(b)
	return complex(ar*br+ai*bi, ar*bi-ai*br)
}
