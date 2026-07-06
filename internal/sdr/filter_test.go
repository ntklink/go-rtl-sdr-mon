package sdr

import (
	"math"
	"math/cmplx"
	"testing"
)

// complexGain feeds a complex exponential at frequency f through a complex FIR
// filter and returns the steady-state magnitude (passband gain), skipping the
// filter's transient (group delay ~ numTaps/2).
func complexGain(taps []complex128, sampleRate, freq float64) float64 {
	filt := NewFIRComplexFilter(taps)
	n := 1 << 14
	w := 2 * math.Pi * freq / sampleRate
	var sumSq float64
	count := 0
	for i := 0; i < n; i++ {
		s := cmplx.Exp(complex(0, w*float64(i)))
		y := filt.Filter(s)
		if i < len(taps) { // skip transient
			continue
		}
		sumSq += real(y)*real(y) + imag(y)*imag(y)
		count++
	}
	if count == 0 {
		return 0
	}
	return math.Sqrt(sumSq / float64(count))
}

// TestComplexBandpassSelectsSingleSideband verifies that the complex bandpass
// used for SSB passes the desired sideband and rejects the mirror sideband
// (a real bandpass cannot do this, which was the original bug).
func TestComplexBandpassSelectsSingleSideband(t *testing.T) {
	const sampleRate = 24000.0
	// USB: pass [100, 4000], reject negative frequencies.
	usb := DesignComplexBandpass(sampleRate, (100+4000)/2, (4000-100)/2, 65)

	inBand := complexGain(usb, sampleRate, 1000)   // +1000 Hz, in passband
	mirror := complexGain(usb, sampleRate, -1000)  // -1000 Hz, must be rejected
	outside := complexGain(usb, sampleRate, 6000)  // +6000 Hz, outside passband

	if mirror > inBand*0.3 {
		t.Errorf("USB failed to reject mirror: inBand=%.3f mirror=%.3f", inBand, mirror)
	}
	if outside > inBand*0.3 {
		t.Errorf("USB failed to reject out-of-band: inBand=%.3f outside=%.3f", inBand, outside)
	}

	// LSB: pass [-4000, -100], reject positive frequencies.
	lsb := DesignComplexBandpass(sampleRate, (-4000+-100)/2, (4000-100)/2, 65)
	inBandLsb := complexGain(lsb, sampleRate, -1000) // -1000 Hz, in passband
	mirrorLsb := complexGain(lsb, sampleRate, 1000)  // +1000 Hz, must be rejected

	if mirrorLsb > inBandLsb*0.3 {
		t.Errorf("LSB failed to reject mirror: inBand=%.3f mirror=%.3f", inBandLsb, mirrorLsb)
	}
}
