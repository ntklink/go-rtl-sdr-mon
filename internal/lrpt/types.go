// Package lrpt implements a Meteor-M LRPT (Low Rate Picture Transmission)
// receiver: QPSK demodulation, convolutional (Viterbi) decoding, CCSDS
// deframing, Reed-Solomon error correction, and MSU-MR JPEG image decoding.
//
// Signal parameters (Meteor-M N2-3 / N2-4):
//   - 137.1 / 137.9 MHz, QPSK, 72000 symbols/s
//   - Convolutional code K=7, r=1/2 (G1=0171, G2=0133 octal)
//   - CADU frames: 32-bit ASM 0x1ACFFC1D + 1020 bytes randomized data
//   - Reed-Solomon (255,223), interleave 4, dual-basis-equivalent
//     parameters (first root 112, root skip 11 over GF(2^8)/0x187)
//   - Payload: CCSDS space packets, APID 64-69 carry 8-bit grayscale
//     image channels compressed with a JPEG-like scheme (8x8 DCT,
//     standard Annex K luminance Huffman tables)
package lrpt

// Satellite describes a Meteor-M satellite transmitting LRPT.
type Satellite struct {
	Name      string  `json:"name"`
	Frequency uint32  `json:"frequency"` // Hz
	Period    float64 `json:"period"`    // orbital period in minutes
	Status    string  `json:"status"`
}

// Satellites returns the list of Meteor-M satellites transmitting LRPT.
func Satellites() []Satellite {
	return []Satellite{
		{Name: "Meteor-M N2-3", Frequency: 137900000, Period: 101.3, Status: "active"},
		{Name: "Meteor-M N2-4", Frequency: 137100000, Period: 101.3, Status: "active"},
	}
}

// Signal / framing constants.
const (
	SymbolRate = 72000.0 // QPSK symbols per second

	// CADU framing
	ASM          = 0x1ACFFC1D // attached sync marker
	CADULen      = 1024       // ASM (4) + data (1020) bytes
	CADUDataLen  = 1020       // randomized, RS-encoded data
	FrameBits    = CADULen * 8
	FrameSymbols = FrameBits // r=1/2: 2 coded bits per input bit, 2 bits/symbol

	// Reed-Solomon (255,223) x4 interleave over the 1020-byte block
	RSBlockLen   = 255
	RSDataLen    = 223
	RSInterleave = 4
	VCDULen      = RSDataLen * RSInterleave // 892

	// VCDU layout
	VCDUHdrLen     = 6
	VCDUInsertLen  = 2 // encryption flag + key
	MPDUHdrLen     = 2
	MPDUDataLen    = 882
	MPDUDataOffset = VCDUHdrLen + VCDUInsertLen + MPDUHdrLen // 10

	// MSU-MR image geometry
	MCUPerLine   = 196            // 8x8 blocks per scan line
	MCUPerPacket = 14             // blocks per CCSDS packet
	ImageWidth   = MCUPerLine * 8 // 1568 px
	StripHeight  = 8              // pixel rows per MCU strip

	// One MCU strip (8 image lines) is scanned every ~1.232 s
	// (MSU-MR scans 6.5 lines/s).
	StripPeriodMs = 1232.0

	// Image APID range for MSU-MR channels
	APIDImageMin = 64
	APIDImageMax = 69
)

// ImageSegment is one decoded packet's worth of image data: 14 MCUs =
// 112x8 pixels of one channel, at horizontal block offset MCUIndex and
// vertical strip index Strip (top row = Strip*8).
type ImageSegment struct {
	APID     int    `json:"apid"`
	Strip    int    `json:"strip"`    // strip index since decoder start/reset
	MCUIndex int    `json:"mcuIndex"` // first MCU index (x0 = MCUIndex*8)
	Pixels   []byte `json:"pixels"`   // 8 rows x 112 cols, row-major
}

// Stats reports decoder state for the UI.
type Stats struct {
	Locked     bool    `json:"locked"`      // carrier+timing+frame lock
	SignalQ    float64 `json:"signalQ"`     // 0-100 signal quality (from EVM)
	FreqOffset float64 `json:"freqOffset"`  // estimated carrier offset, Hz
	FramesOK   int     `json:"framesOK"`    // CADUs passing ASM+RS
	FramesBad  int     `json:"framesBad"`   // sync hits failing ASM/RS
	RSCorrect  int     `json:"rsCorrected"` // total RS-corrected bytes
	Packets    int     `json:"packets"`     // image packets decoded
	APIDs      []int   `json:"apids"`       // image APIDs seen this session
}
