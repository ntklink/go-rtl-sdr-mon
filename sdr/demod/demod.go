package demod

// DemodType identifies a demodulator type.
type DemodType int

const (
	DemodNone DemodType = iota
	DemodAM
	DemodNFM // narrow FM
	DemodWFM // wide FM (broadcast)
	DemodWFMStereo
	DemodSSB // SSB (USB/LSB selected by filter offset sign)
	DemodAMSync
)

// String returns the name of the demodulator type.
func (d DemodType) String() string {
	switch d {
	case DemodNone:
		return "NONE"
	case DemodAM:
		return "AM"
	case DemodNFM:
		return "NFM"
	case DemodWFM:
		return "WFM"
	case DemodWFMStereo:
		return "WFM-Stereo"
	case DemodSSB:
		return "SSB"
	case DemodAMSync:
		return "AM-Sync"
	default:
		return "Unknown"
	}
}

// Demodulator is the interface that all demodulators implement.
// Input: complex128 IQ samples at the quadrature rate.
// Output: float64 audio samples (mono = ch0, stereo = ch0+ch1).
type Demodulator interface {
	// Process demodulates a block of IQ samples.
	// Returns (left, right) audio channels. For mono, right is nil.
	Process(in []complex128) (left, right []float64)

	// Type returns the demodulator type.
	Type() DemodType

	// SetQuadRate updates the quadrature sample rate.
	SetQuadRate(rate float64)

	// QuadRate returns the current quadrature rate.
	QuadRate() float64
}
