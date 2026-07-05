package sdr

import (
	"math"
)

// AGC is an Automatic Gain Controller for audio signals.
type AGC struct {
	enabled    bool
	hang       bool
	threshold  float64 // in dB
	slope      float64 // dB
	decay      float64 // decay rate (ms)
	manualGain float64

	// State
	gain        float64
	peakHold    float64
	holdCounter int
	decayRate   float64
	sampleRate  float64
}

// NewAGC creates a new AGC with the given sample rate.
func NewAGC(sampleRate float64) *AGC {
	return &AGC{
		enabled:    false,
		threshold:  -20,
		slope:      2,
		decay:      250,
		manualGain: 1.0,
		gain:       1.0,
		peakHold:   0,
		sampleRate: sampleRate,
	}
}

// SetEnabled enables or disables the AGC.
func (a *AGC) SetEnabled(on bool) {
	a.enabled = on
	if !on {
		a.gain = a.manualGain
	}
}

// SetHang enables or disables hang mode.
func (a *AGC) SetHang(on bool) {
	a.hang = on
}

// SetThreshold sets the AGC threshold in dB.
func (a *AGC) SetThreshold(threshold float64) {
	a.threshold = threshold
}

// SetSlope sets the AGC slope in dB.
func (a *AGC) SetSlope(slope float64) {
	a.slope = slope
}

// SetDecay sets the AGC decay rate in milliseconds.
func (a *AGC) SetDecay(decay float64) {
	a.decay = decay
	a.decayRate = math.Exp(-1.0 / (a.sampleRate * a.decay / 1000.0))
}

// SetManualGain sets the manual gain (used when AGC is off).
func (a *AGC) SetManualGain(gain float64) {
	a.manualGain = gain
	if !a.enabled {
		a.gain = gain
	}
}

// Process applies AGC to a slice of float samples and returns the result.
func (a *AGC) Process(in []float64) []float64 {
	if !a.enabled {
		g := a.manualGain
		out := make([]float64, len(in))
		for i, x := range in {
			out[i] = x * g
		}
		return out
	}

	out := make([]float64, len(in))
	holdTime := int(a.sampleRate * 0.1) // 100ms hold

	for i, x := range in {
		// Rectify
		absX := math.Abs(x)

		// Peak detect with hang
		if absX > a.peakHold {
			a.peakHold = absX
			a.holdCounter = 0
		} else {
			a.holdCounter++
			if a.holdCounter > holdTime {
				a.peakHold *= a.decayRate
			}
		}

		// Compute target gain
		if a.peakHold > 1e-10 {
			levelDB := 20 * math.Log10(a.peakHold)
			if levelDB > a.threshold {
				targetDB := a.slope * (levelDB - a.threshold)
				targetGain := math.Pow(10, -targetDB/20.0)
				// Smoothly approach target gain
				a.gain = 0.99*a.gain + 0.01*targetGain
			}
		}

		// Clamp gain
		if a.gain > 1e6 {
			a.gain = 1e6
		}
		if a.gain < 0 {
			a.gain = 0
		}

		out[i] = x * a.gain
	}

	return out
}
