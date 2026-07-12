package lrpt

import "math/bits"

// Deframer turns the soft symbol stream into verified 892-byte VCDUs:
// ASM correlation (with QPSK phase-ambiguity resolution) → Viterbi →
// ASM check → PN derandomization → Reed-Solomon.
//
// The 32-bit ASM is convolutionally encoded into 64 coded bits; the
// first 12 depend on the (unknown) encoder state from the previous
// frame, so the correlator matches the remaining 52 bits, i.e. 26 QPSK
// symbols starting at symbol 6 of the frame. QPSK has an 8-fold
// ambiguity (4 phase rotations × spectral inversion); one hard-bit
// pattern is precomputed per ambiguity and the matching transform is
// applied to the soft symbols before Viterbi decoding.

const (
	corrSkipSyms  = 6  // coded symbols of the ASM affected by prior state
	corrSyms      = 26 // symbols correlated (52 coded bits)
	corrMaxErrors = 5  // max mismatched bits to accept a sync candidate

	// Soft buffer: 2 frames + slack, in symbols
	symBufLen = 3 * FrameSymbols

	frameTailBits = 32 // extra bits decoded past the frame for traceback
)

// ambTransform applies ambiguity k (0-3: rotate k×90°; +4: conjugate
// then rotate) to a soft symbol pair.
func ambTransform(k int, i, q int8) (int8, int8) {
	if k >= 4 {
		i, q = q, i // conjugate+swap: mirror across the I=Q diagonal
		k -= 4
	}
	switch k {
	case 1:
		return negS(q), i
	case 2:
		return negS(i), negS(q)
	case 3:
		return q, negS(i)
	}
	return i, q
}

func negS(v int8) int8 {
	if v == -128 {
		return 127
	}
	return -v
}

type deframer struct {
	vit *viterbi
	rs  *rsCodec

	// PN sequence covering the 1020-byte randomized zone
	pn [CADUDataLen]byte

	// ambiguity patterns: for each of 8 transforms, the expected hard
	// bits of coded-ASM symbols 6..31 packed as two 26-bit words
	patI [8]uint32
	patQ [8]uint32

	// rolling hard-bit registers of the most recent 26 symbols
	regI uint32
	regQ uint32

	// soft symbol ring buffer
	buf   []int8 // interleaved I,Q
	nSyms int    // symbols currently buffered

	// scan position within the buffer (symbols already searched)
	scanPos int

	// pending sync candidate waiting for a full frame of symbols
	pendPos int // symbol offset of the frame start, -1 = none
	pendAmb int

	frameSoft []int8
	frameOut  []byte

	// stats
	framesOK  int
	framesBad int
	rsCorr    int
}

func newDeframer() *deframer {
	d := &deframer{
		vit:       newViterbi(),
		rs:        newRSCodec(),
		buf:       make([]int8, 0, 2*symBufLen),
		frameSoft: make([]int8, 2*(FrameBits+frameTailBits)),
		frameOut:  make([]byte, CADULen+frameTailBits/8),
		pendPos:   -1,
	}

	// PN sequence: x^8+x^7+x^5+x^3+1, seed 0xFF, MSB out
	state := 0xFF
	for i := range d.pn {
		var b byte
		for k := 0; k < 8; k++ {
			b = b<<1 | byte(state>>7&1)
			fb := bits.OnesCount8(uint8(state&0x95)) & 1
			state = (state<<1 | fb) & 0xFF
		}
		d.pn[i] = b
	}

	// Encoded ASM reference (state 0; first corrSkipSyms symbols are
	// state-dependent and excluded from the patterns).
	asm := []byte{0x1A, 0xCF, 0xFC, 0x1D}
	coded, _ := ConvEncode(asm, 0)
	for amb := range 8 {
		var pi, pq uint32
		for s := corrSkipSyms; s < corrSkipSyms+corrSyms; s++ {
			// reference soft signs for bit values
			i := int8(1 - 2*int(coded[2*s])) // +1 for bit0, -1 for bit1
			q := int8(1 - 2*int(coded[2*s+1]))
			// The stream equals the reference with transform amb
			// applied; store the transformed pattern.
			ti, tq := ambTransform(amb, i, q)
			pi = pi<<1 | uint32(1-int(ti))>>1 // -1 → bit 1, +1 → bit 0
			pq = pq<<1 | uint32(1-int(tq))>>1
		}
		d.patI[amb] = pi
		d.patQ[amb] = pq
	}
	return d
}

// inverseAmb returns the transform that undoes ambiguity k.
func inverseAmb(k int) int {
	switch k {
	case 1:
		return 3
	case 3:
		return 1
	}
	return k // 0, 2 and all conjugate variants are self-inverse
}

func (d *deframer) reset() {
	d.buf = d.buf[:0]
	d.nSyms = 0
	d.scanPos = 0
	d.pendPos = -1
	d.regI, d.regQ = 0, 0
	d.framesOK, d.framesBad, d.rsCorr = 0, 0, 0
}

// process consumes soft symbols and returns any verified VCDUs (each
// exactly VCDULen bytes, caller must copy if retained across calls).
func (d *deframer) process(soft []int8) [][]byte {
	var vcdus [][]byte
	d.buf = append(d.buf, soft...)
	d.nSyms = len(d.buf) / 2

	const mask = (1 << corrSyms) - 1

	for {
		// Scan for a sync candidate unless one is already pending
		if d.pendPos < 0 {
			for ; d.scanPos < d.nSyms; d.scanPos++ {
				i := d.buf[2*d.scanPos]
				q := d.buf[2*d.scanPos+1]
				d.regI = (d.regI<<1 | uint32(1-int(sign8(i)))>>1) & mask
				d.regQ = (d.regQ<<1 | uint32(1-int(sign8(q)))>>1) & mask
				if d.scanPos < corrSyms-1 {
					continue
				}
				for amb := range 8 {
					errs := bits.OnesCount32(d.regI^d.patI[amb]) +
						bits.OnesCount32(d.regQ^d.patQ[amb])
					if errs <= corrMaxErrors {
						pos := d.scanPos - (corrSyms - 1) - corrSkipSyms
						if pos >= 0 {
							d.pendPos = pos
							d.pendAmb = amb
						}
						break
					}
				}
				if d.pendPos >= 0 {
					d.scanPos++
					break
				}
			}
			if d.pendPos < 0 {
				d.prune()
				return vcdus
			}
		}

		// Wait until the full frame plus traceback tail is buffered
		if d.pendPos+FrameBits+frameTailBits > d.nSyms {
			d.prune()
			return vcdus
		}

		pos, amb := d.pendPos, d.pendAmb
		d.pendPos = -1
		if v := d.decodeFrame(pos, amb); v != nil {
			vcdus = append(vcdus, v)
			// Skip the scan to the end of this frame and rebuild the
			// correlation registers from the preceding symbols.
			d.scanPos = pos + FrameBits
			d.regI, d.regQ = 0, 0
			start := d.scanPos - (corrSyms - 1)
			if start < 0 {
				start = 0
			}
			for p := start; p < d.scanPos; p++ {
				d.regI = (d.regI<<1 | uint32(1-int(sign8(d.buf[2*p])))>>1) & mask
				d.regQ = (d.regQ<<1 | uint32(1-int(sign8(d.buf[2*p+1])))>>1) & mask
			}
		}
		d.prune()
	}
}

// decodeFrame Viterbi-decodes one frame candidate at symbol offset pos,
// verifies the ASM, derandomizes and RS-corrects it. Returns the VCDU
// or nil.
func (d *deframer) decodeFrame(pos, amb int) []byte {
	inv := inverseAmb(amb)
	n := FrameBits + frameTailBits
	if pos+n > d.nSyms {
		n = d.nSyms - pos
	}
	for s := range n {
		i := d.buf[2*(pos+s)]
		q := d.buf[2*(pos+s)+1]
		ti, tq := ambTransform(inv, i, q)
		d.frameSoft[2*s] = ti
		d.frameSoft[2*s+1] = tq
	}

	out := d.frameOut
	d.vit.decode(d.frameSoft[:2*n], n, out[:CADULen])

	// Verify ASM (tolerate a couple of bit errors from the unconverged
	// start of the traceback)
	asmWord := uint32(out[0])<<24 | uint32(out[1])<<16 | uint32(out[2])<<8 | uint32(out[3])
	if bits.OnesCount32(asmWord^ASM) > 4 {
		d.framesBad++
		return nil
	}

	// Derandomize
	data := out[4:CADULen]
	for i := range data {
		data[i] ^= d.pn[i]
	}

	// Deinterleave + RS decode 4 codewords
	var cw [RSBlockLen]byte
	vcdu := make([]byte, VCDULen)
	totalCorr := 0
	for i := range RSInterleave {
		for j := range RSBlockLen {
			cw[j] = data[j*RSInterleave+i]
		}
		nc := d.rs.decode(cw[:])
		if nc < 0 {
			d.framesBad++
			return nil
		}
		totalCorr += nc
		for j := range RSDataLen {
			vcdu[j*RSInterleave+i] = cw[j]
		}
	}

	d.rsCorr += totalCorr
	d.framesOK++
	return vcdu
}

// prune drops consumed symbols, keeping enough history for the scan
// window and any pending frame candidate.
func (d *deframer) prune() {
	keepFrom := d.scanPos - (corrSyms + corrSkipSyms)
	if d.pendPos >= 0 && d.pendPos < keepFrom {
		keepFrom = d.pendPos
	}
	// Hard cap on buffer growth: drop oldest data even if pending
	// (the candidate is then abandoned).
	if excess := d.nSyms - symBufLen; excess > 0 && keepFrom < excess {
		keepFrom = excess
		if d.pendPos >= 0 && d.pendPos < keepFrom {
			d.pendPos = -1
		}
	}
	if keepFrom <= 0 {
		return
	}
	d.buf = append(d.buf[:0], d.buf[2*keepFrom:]...)
	d.nSyms -= keepFrom
	d.scanPos -= keepFrom
	if d.pendPos >= 0 {
		d.pendPos -= keepFrom
	}
}

func sign8(v int8) int8 {
	if v < 0 {
		return -1
	}
	return 1
}
