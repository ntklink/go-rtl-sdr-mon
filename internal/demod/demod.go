package demod

// DemodType identifies a demodulator type.
// Values and order match gqrx's DockRxOpt::rxopt_mode_idx enum.
type DemodType int

const (
	DemodOff       DemodType = iota // 0  - Demod Off
	DemodRaw                        // 1  - Raw I/Q (no demod)
	DemodAM                         // 2  - AM
	DemodAMSync                     // 3  - AM-Sync
	DemodLSB                        // 4  - LSB (Lower Sideband)
	DemodUSB                        // 5  - USB (Upper Sideband)
	DemodCWL                        // 6  - CW-L (CW Lower)
	DemodCWU                        // 7  - CW-U (CW Upper)
	DemodNFM                        // 8  - Narrow FM
	DemodWFM                        // 9  - WFM (mono)
	DemodWFMStereo                  // 10 - WFM (stereo)
	DemodWFMOirt                    // 11 - WFM (oirt stereo)
	DemodADSB                       // 12 - ADS-B (1090 MHz)
	DemodNOAA                       // 13 - NOAA APT (137 MHz)
)

// String returns the name of the demodulator type.
func (d DemodType) String() string {
	switch d {
	case DemodOff:
		return "OFF"
	case DemodRaw:
		return "Raw I/Q"
	case DemodAM:
		return "AM"
	case DemodAMSync:
		return "AM-Sync"
	case DemodLSB:
		return "LSB"
	case DemodUSB:
		return "USB"
	case DemodCWL:
		return "CW-L"
	case DemodCWU:
		return "CW-U"
	case DemodNFM:
		return "NFM"
	case DemodWFM:
		return "WFM"
	case DemodWFMStereo:
		return "WFM-Stereo"
	case DemodWFMOirt:
		return "WFM-OIRT"
	case DemodADSB:
		return "ADS-B"
	case DemodNOAA:
		return "NOAA"
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
