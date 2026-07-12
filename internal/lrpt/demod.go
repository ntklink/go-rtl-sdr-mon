package lrpt

import (
	"math"
	"math/cmplx"
)

const (
	rrcAlpha = 0.6 // LRPT root-raised-cosine rolloff
	rrcSpan  = 8   // filter span in symbols

	acqFFTSize = 4096 // coarse carrier acquisition FFT length
)

// Demodulator recovers QPSK soft symbols from complex baseband IQ.
//
// Chain: AGC → carrier NCO (Costas loop) → RRC matched filter →
// fractional-delay half-symbol ticks with a Gardner timing loop →
// soft symbols (int8 I,Q pairs).
//
// Large tuning offsets (RTL-SDR ppm error + doppler, up to several kHz)
// are handled by a coarse acquisition stage: while unlocked, the 4th
// power of the signal (which collapses QPSK modulation to a carrier at
// 4×Δf) is run through an FFT and the peak sets the NCO frequency.
type Demodulator struct {
	fs      float64
	sps     float64 // samples per symbol
	halfSps float64

	agcMag float64 // EMA of |x|

	// Carrier NCO + 2nd-order Costas loop (gains applied per symbol)
	phase  float64
	freq   float64 // rad/sample
	alphaC float64
	betaC  float64

	// RRC matched filter (computed at every input sample)
	taps  []float64
	dl    []complex128
	pos   int
	prevY complex128

	// Gardner timing: ticks alternate midpoint / decision
	tickPhase      float64 // samples until next half-symbol tick (counts down)
	tickIsDecision bool
	midSample      complex128
	prevDecision   complex128
	muGain         float64 // timing proportional gain (per decision)
	muGainInt      float64 // timing integral gain
	spsAdj         float64 // accumulated timing rate correction

	// Symbol magnitude and quality tracking
	magSym float64 // EMA of decision magnitude
	evm    float64 // EMA of normalized error-vector magnitude (0..~1.4)

	// Coarse acquisition
	acqBuf  []complex128
	acqSkip int

	// Constellation snapshot for the UI (ring of recent soft pairs)
	constRing [256]int8
	constPos  int

	out []int8
}

// NewDemodulator creates a QPSK demodulator for the given input rate.
func NewDemodulator(sampleRate float64) *Demodulator {
	d := &Demodulator{
		fs:      sampleRate,
		sps:     sampleRate / SymbolRate,
		halfSps: sampleRate / SymbolRate / 2,
		agcMag:  1,
		magSym:  1,
		evm:     1,
	}

	// Costas loop: Bn ≈ 110 Hz at the 72k symbol rate, ζ = 0.707.
	const zeta = 0.707
	bnT := 110.0 / SymbolRate
	theta := bnT / (zeta + 1/(4*zeta))
	denom := 1 + 2*zeta*theta + theta*theta
	d.alphaC = 4 * zeta * theta / denom
	d.betaC = 4 * theta * theta / denom

	// Gardner timing loop gains (per decision tick).
	d.muGain = 0.05
	d.muGainInt = 5e-5

	d.taps = rrcTaps(d.sps)
	d.dl = make([]complex128, len(d.taps))
	d.tickPhase = d.halfSps
	d.acqBuf = make([]complex128, 0, acqFFTSize)
	return d
}

// rrcPulse evaluates the root-raised-cosine impulse response (rolloff
// rrcAlpha) at time t in symbol periods.
func rrcPulse(t float64) float64 {
	a := rrcAlpha
	switch {
	case t == 0:
		return 1 - a + 4*a/math.Pi
	case math.Abs(math.Abs(t)-1/(4*a)) < 1e-9:
		return a / math.Sqrt2 * ((1+2/math.Pi)*math.Sin(math.Pi/(4*a)) +
			(1-2/math.Pi)*math.Cos(math.Pi/(4*a)))
	default:
		return (math.Sin(math.Pi*t*(1-a)) + 4*a*t*math.Cos(math.Pi*t*(1+a))) /
			(math.Pi * t * (1 - 16*a*a*t*t))
	}
}

// rrcTaps computes root-raised-cosine taps spanning rrcSpan symbols at
// the given samples-per-symbol.
func rrcTaps(sps float64) []float64 {
	n := int(rrcSpan*sps) | 1 // odd length
	taps := make([]float64, n)
	mid := n / 2
	var sum float64
	for i := range taps {
		taps[i] = rrcPulse(float64(i-mid) / sps)
		sum += taps[i]
	}
	for i := range taps {
		taps[i] /= sum
	}
	return taps
}

// Unlocked QPSK (freely rotating constellation) measures an EVM of
// ≈0.39; a locked signal at usable SNR sits below ~0.3.
const evmLocked = 0.36

// Locked reports whether the carrier/timing loops have converged.
func (d *Demodulator) Locked() bool { return d.evm < evmLocked }

// Quality returns a 0-100 signal quality figure derived from the EVM.
func (d *Demodulator) Quality() float64 {
	q := (1 - d.evm/0.45) * 100
	if q < 0 {
		q = 0
	}
	if q > 100 {
		q = 100
	}
	return q
}

// FreqOffset returns the current carrier offset estimate in Hz.
func (d *Demodulator) FreqOffset() float64 {
	return d.freq * d.fs / (2 * math.Pi)
}

// Constellation returns a snapshot of recent soft symbols (I,Q pairs).
func (d *Demodulator) Constellation() []int8 {
	out := make([]int8, len(d.constRing))
	copy(out, d.constRing[:])
	return out
}

// Reset clears all demodulator state.
func (d *Demodulator) Reset() {
	d.phase, d.freq = 0, 0
	d.agcMag, d.magSym, d.evm = 1, 1, 1
	d.spsAdj = 0
	d.tickPhase = d.halfSps
	d.tickIsDecision = false
	d.prevY, d.midSample, d.prevDecision = 0, 0, 0
	for i := range d.dl {
		d.dl[i] = 0
	}
	d.acqBuf = d.acqBuf[:0]
	d.acqSkip = 0
	for i := range d.constRing {
		d.constRing[i] = 0
	}
}

// Process demodulates a block of IQ samples and returns soft symbols as
// interleaved int8 I,Q pairs. The returned slice is reused across calls.
func (d *Demodulator) Process(iq []complex128) []int8 {
	d.out = d.out[:0]
	nTaps := len(d.taps)

	for _, x := range iq {
		// AGC to ~unit magnitude
		m := cmplx.Abs(x)
		d.agcMag = d.agcMag*0.9999 + m*0.0001
		if d.agcMag > 1e-9 {
			x /= complex(d.agcMag, 0)
		}

		// Coarse carrier tracking (always on: compares the 4th-power
		// spectral peak against the current NCO and snaps on mismatch)
		d.coarseAcquire(x)

		// Carrier NCO
		x *= cmplx.Exp(complex(0, -d.phase))
		d.phase += d.freq
		if d.phase > 2*math.Pi {
			d.phase -= 2 * math.Pi
		} else if d.phase < -2*math.Pi {
			d.phase += 2 * math.Pi
		}

		// RRC matched filter (circular delay line)
		d.dl[d.pos] = x
		var y complex128
		idx := d.pos
		for _, t := range d.taps {
			y += complex(t, 0) * d.dl[idx]
			idx--
			if idx < 0 {
				idx = nTaps - 1
			}
		}
		d.pos++
		if d.pos >= nTaps {
			d.pos = 0
		}

		// Half-symbol ticks with linear interpolation between the
		// previous and current filter outputs.
		d.tickPhase--
		for d.tickPhase <= 0 {
			frac := d.tickPhase + 1 // position in (0,1] between prevY and y
			yt := d.prevY + complex(frac, 0)*(y-d.prevY)
			d.tick(yt)
			d.tickPhase += d.halfSps + d.spsAdj
		}
		d.prevY = y
	}

	return d.out
}

// tick handles one half-symbol tick: midpoint samples feed the Gardner
// detector, decision samples close the timing and Costas loops and emit
// a soft symbol.
func (d *Demodulator) tick(y complex128) {
	if !d.tickIsDecision {
		d.midSample = y
		d.tickIsDecision = true
		return
	}
	d.tickIsDecision = false

	i, q := real(y), imag(y)

	// Track symbol magnitude for normalization
	mag := math.Hypot(i, q)
	d.magSym = d.magSym*0.995 + mag*0.005
	norm := d.magSym
	if norm < 1e-9 {
		norm = 1e-9
	}

	// Gardner timing error detector
	te := real((d.prevDecision - y) * cmplx.Conj(d.midSample))
	te /= norm * norm
	if te > 1 {
		te = 1
	} else if te < -1 {
		te = -1
	}
	d.tickPhase += d.muGain * te
	d.spsAdj += d.muGainInt * te
	// Bound the rate correction to ±1% of the nominal half-symbol period
	if lim := d.halfSps * 0.01; d.spsAdj > lim {
		d.spsAdj = lim
	} else if lim := -d.halfSps * 0.01; d.spsAdj < lim {
		d.spsAdj = lim
	}
	d.prevDecision = y

	// Costas phase error (decision-directed QPSK)
	var pe float64
	if i != 0 || q != 0 {
		pe = (sign(i)*q - sign(q)*i) / norm
	}
	d.freq += d.betaC * pe / d.sps
	d.phase += d.alphaC * pe
	// Bound NCO frequency to ±10 kHz
	if lim := 2 * math.Pi * 10000 / d.fs; d.freq > lim {
		d.freq = lim
	} else if d.freq < -2*math.Pi*10000/d.fs {
		d.freq = -2 * math.Pi * 10000 / d.fs
	}

	// EVM against the nearest ideal constellation point
	ideal := norm / math.Sqrt2
	ei := math.Abs(i) - ideal
	eq := math.Abs(q) - ideal
	ev := math.Sqrt(ei*ei+eq*eq) / norm
	d.evm = d.evm*0.998 + ev*0.002

	// Emit soft symbol
	si := clampSoft(i / norm * 96)
	sq := clampSoft(q / norm * 96)
	d.out = append(d.out, si, sq)
	d.constRing[d.constPos] = si
	d.constRing[d.constPos+1] = sq
	d.constPos = (d.constPos + 2) % len(d.constRing)
}

// coarseAcquire accumulates samples and, once enough are collected,
// estimates the absolute carrier offset from the peak of FFT(x^4). If
// the estimate disagrees with the current NCO frequency by more than
// the Costas pull-in range, the NCO is snapped to it; while locked the
// estimate matches the NCO and nothing happens.
func (d *Demodulator) coarseAcquire(x complex128) {
	// Collect every other sample to halve the FFT cadence cost
	d.acqSkip++
	if d.acqSkip&1 == 0 {
		return
	}
	x4 := x * x
	x4 *= x4
	d.acqBuf = append(d.acqBuf, x4)
	if len(d.acqBuf) < acqFFTSize {
		return
	}

	fftInPlace(d.acqBuf)
	best, bestPow := 0, 0.0
	for k, v := range d.acqBuf {
		p := real(v)*real(v) + imag(v)*imag(v)
		if p > bestPow {
			bestPow = p
			best = k
		}
	}
	// Effective rate is fs/2 because of the decimation above
	effFs := d.fs / 2
	bin := float64(best)
	if bin > acqFFTSize/2 {
		bin -= acqFFTSize
	}
	offset4 := bin * effFs / acqFFTSize // frequency of the 4th-power tone
	offset := offset4 / 4
	cur := d.freq * d.fs / (2 * math.Pi)
	if math.Abs(offset) < 10000 && math.Abs(offset-cur) > 30 {
		d.freq = 2 * math.Pi * offset / d.fs
	}
	d.acqBuf = d.acqBuf[:0]
}

// fftInPlace computes an in-place radix-2 FFT (len must be a power of 2).
func fftInPlace(a []complex128) {
	n := len(a)
	// bit reversal
	for i, j := 0, 0; i < n; i++ {
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
		mask := n >> 1
		for j&mask != 0 {
			j &^= mask
			mask >>= 1
		}
		j |= mask
	}
	for size := 2; size <= n; size <<= 1 {
		step := -2 * math.Pi / float64(size)
		half := size / 2
		for start := 0; start < n; start += size {
			for k := range half {
				w := cmplx.Exp(complex(0, step*float64(k)))
				u := a[start+k]
				v := a[start+k+half] * w
				a[start+k] = u + v
				a[start+k+half] = u - v
			}
		}
	}
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

func clampSoft(v float64) int8 {
	if v > 127 {
		return 127
	}
	if v < -127 {
		return -127
	}
	return int8(v)
}
