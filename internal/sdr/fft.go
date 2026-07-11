package sdr

import (
	"math"
	"time"
)

// SpectrumFFT computes magnitude spectra for waterfall/spectrum display.
type SpectrumFFT struct {
	size     int
	window   []float64
	prevMag  []float64 // for averaging
	avg      float64   // averaging factor (0..1), higher = more averaging
	maxHold  bool      // max-hold plot mode
	maxMag   []float64 // max-hold buffer
	maxDecay float64   // max-hold decay rate (dB per second)
	lastTime float64   // timestamp of last compute (for decay calc)

	// paddedBuf/windowedBuf are scratch space reused across Compute calls;
	// unlike the returned magnitude slice (broadcast to multiple slow
	// WebSocket subscribers and so kept a fresh allocation per call), these
	// never leave this function, so reusing them is safe.
	paddedBuf   []complex128
	windowedBuf []complex128
}

// NewSpectrumFFT creates a new FFT processor with the given size.
// A Hann window is applied before the FFT.
func NewSpectrumFFT(size int, avg float64) *SpectrumFFT {
	if size <= 0 {
		size = 8192
	}
	// Round to next power of 2
	size = nextPow2(size)

	w := make([]float64, size)
	for i := range w {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(size-1))
	}

	return &SpectrumFFT{
		size:     size,
		window:   w,
		prevMag:  make([]float64, size),
		maxMag:   make([]float64, size),
		avg:      avg,
		maxDecay: 6.0, // 6 dB/s decay
	}
}

// Size returns the FFT size.
func (f *SpectrumFFT) Size() int {
	return f.size
}

// SetAvg sets the FFT averaging factor (0 = no averaging, 0.9 = heavy).
func (f *SpectrumFFT) SetAvg(avg float64) {
	if avg < 0 {
		avg = 0
	}
	if avg > 0.99 {
		avg = 0.99
	}
	f.avg = avg
}

// SetMaxHold enables or disables max-hold plot mode.
func (f *SpectrumFFT) SetMaxHold(on bool) {
	f.maxHold = on
	if !on {
		for i := range f.maxMag {
			f.maxMag[i] = 0
		}
	}
}

// Compute computes the power spectrum in dBFS from complex samples.
// Returns a slice of length size (the full spectrum, fftshifted so that DC
// is in the center, covering -fs/2 .. +fs/2).
func (f *SpectrumFFT) Compute(samples []complex128) []float32 {
	n := f.size
	if len(samples) < n {
		// pad with zeros
		if cap(f.paddedBuf) < n {
			f.paddedBuf = make([]complex128, n)
		}
		padded := f.paddedBuf[:n]
		for i := len(samples); i < n; i++ {
			padded[i] = 0
		}
		copy(padded, samples)
		samples = padded
		f.paddedBuf = padded
	}

	// Apply window and take first n samples
	if cap(f.windowedBuf) < n {
		f.windowedBuf = make([]complex128, n)
	}
	windowed := f.windowedBuf[:n]
	for i := 0; i < n; i++ {
		windowed[i] = complex(real(samples[i])*f.window[i], imag(samples[i])*f.window[i])
	}
	f.windowedBuf = windowed

	// Compute FFT (complex FFT using radix-2 Cooley-Tukey)
	coeffs := fftComplex(windowed)

	// Compute magnitude in dBFS, with fftshift.
	// out is broadcast by reference to every connected FFT WebSocket
	// subscriber; a slow client may still be reading a prior call's out
	// when the next block arrives, so this one must stay freshly
	// allocated rather than reused.
	out := make([]float32, n)
	half := n / 2
	norm := 1.0 / float64(n)
	now := float64(time.Now().UnixNano()) / 1e9
	dt := 0.0
	if f.lastTime > 0 {
		dt = now - f.lastTime
	}
	f.lastTime = now

	for i := 0; i < n; i++ {
		mag := cmplxAbs(coeffs[i]) * norm
		db := 20.0 * math.Log10(mag+1e-12)
		// fftshift: move upper half to beginning
		idx := (i + half) % n
		// Apply averaging
		f.prevMag[idx] = f.avg*f.prevMag[idx] + (1-f.avg)*db

		if f.maxHold {
			// Decay previous max
			f.maxMag[idx] -= f.maxDecay * dt
			// Keep max
			if f.prevMag[idx] > f.maxMag[idx] {
				f.maxMag[idx] = f.prevMag[idx]
			}
			out[idx] = float32(f.maxMag[idx])
		} else {
			out[idx] = float32(f.prevMag[idx])
		}
	}

	// Return only the positive half (already shifted, so take full)
	return out
}

// nextPow2 rounds n up to the next power of 2.
func nextPow2(n int) int {
	if n <= 0 {
		return 1
	}
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// cmplxAbs returns the magnitude of a complex number.
func cmplxAbs(c complex128) float64 {
	return math.Sqrt(real(c)*real(c) + imag(c)*imag(c))
}

// fftComplex performs an in-place radix-2 Cooley-Tukey FFT on complex128 data.
// The input length must be a power of 2.
func fftComplex(a []complex128) []complex128 {
	n := len(a)
	if n <= 1 {
		return a
	}

	// Bit-reversal permutation
	j := 0
	for i := 1; i < n; i++ {
		bit := n >> 1
		for j&bit != 0 {
			j ^= bit
			bit >>= 1
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}

	// Cooley-Tukey FFT
	for length := 2; length <= n; length <<= 1 {
		halfLen := length >> 1
		angle := -2 * math.Pi / float64(length)
		w := complex(math.Cos(angle), math.Sin(angle))

		for i := 0; i < n; i += length {
			wk := complex(1, 0)
			for k := 0; k < halfLen; k++ {
				t := wk * a[i+k+halfLen]
				u := a[i+k]
				a[i+k] = u + t
				a[i+k+halfLen] = u - t
				wk *= w
			}
		}
	}

	return a
}
