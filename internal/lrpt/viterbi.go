package lrpt

// Convolutional code: K=7, r=1/2, CCSDS polynomials G1=0171, G2=0133
// (octal). Encoder convention: the 6-bit state holds the last 6 input
// bits; for input bit b, full = b<<6 | state, outputs are
// parity(full&G1), parity(full&G2), and the state becomes full>>1.
// Each QPSK symbol carries (G1 out → I, G2 out → Q); bit 0 maps to a
// positive soft value.
const (
	convG1     = 0x79 // 0171 octal
	convG2     = 0x5B // 0133 octal
	convK      = 7
	convStates = 1 << (convK - 1) // 64
)

func parity(v int) int {
	v ^= v >> 4
	v ^= v >> 2
	v ^= v >> 1
	return v & 1
}

// ConvEncode encodes bytes (MSB first) into coded bit pairs, one byte per
// coded bit (0/1), starting from the given encoder state. Returns the
// coded bits and the final state. Used for tests and to generate the
// correlator's ASM reference pattern.
func ConvEncode(data []byte, state int) (bits []byte, endState int) {
	bits = make([]byte, 0, len(data)*16)
	for _, by := range data {
		for k := 7; k >= 0; k-- {
			b := int(by>>uint(k)) & 1
			full := b<<6 | state
			bits = append(bits, byte(parity(full&convG1)), byte(parity(full&convG2)))
			state = full >> 1
		}
	}
	return bits, state
}

// viterbi is a frame-based soft-decision decoder for the K=7 r=1/2 code.
type viterbi struct {
	metrics []int32 // current path metrics [64]
	next    []int32
	// decisions[t*64+s] = chosen "full" (7-bit) leading into state s at step t
	decisions []uint8
	// precomputed per-state transition outputs, as soft-signs (-1/+1)
	outI [128]int32 // for full value f: expected I sign
	outQ [128]int32
}

func newViterbi() *viterbi {
	v := &viterbi{
		metrics: make([]int32, convStates),
		next:    make([]int32, convStates),
	}
	for f := range 128 {
		// bit 0 → +, bit 1 → −
		v.outI[f] = 1 - 2*int32(parity(f&convG1))
		v.outQ[f] = 1 - 2*int32(parity(f&convG2))
	}
	return v
}

// decode runs soft-decision Viterbi over soft symbol pairs (I,Q int8,
// one pair per input bit) and writes the decoded bits MSB-first into out.
// len(soft) must be 2*nbits; out must hold nbits/8 bytes... nbits may
// exceed 8*len(out): extra trailing bits are decoded (improving traceback
// convergence for the last real bits) but not emitted.
func (v *viterbi) decode(soft []int8, nbits int, out []byte) {
	if len(v.decisions) < nbits*convStates {
		v.decisions = make([]uint8, nbits*convStates)
	}
	for s := range v.metrics {
		v.metrics[s] = 0
	}

	for t := range nbits {
		x := int32(soft[2*t])
		y := int32(soft[2*t+1])
		dec := v.decisions[t*convStates : (t+1)*convStates]
		for ns := range convStates {
			f0 := ns << 1 // two candidate "full" values leading to ns
			f1 := f0 | 1
			// predecessor state = full & 63
			m0 := v.metrics[f0&63] + v.outI[f0]*x + v.outQ[f0]*y
			m1 := v.metrics[f1&63] + v.outI[f1]*x + v.outQ[f1]*y
			if m0 >= m1 {
				v.next[ns] = m0
				dec[ns] = uint8(f0)
			} else {
				v.next[ns] = m1
				dec[ns] = uint8(f1)
			}
		}
		v.metrics, v.next = v.next, v.metrics
		// Prevent metric overflow on long frames
		if t&1023 == 1023 {
			min := v.metrics[0]
			for _, m := range v.metrics[1:] {
				if m < min {
					min = m
				}
			}
			for s := range v.metrics {
				v.metrics[s] -= min
			}
		}
	}

	// Traceback from the best final state
	best, bestM := 0, v.metrics[0]
	for s := 1; s < convStates; s++ {
		if v.metrics[s] > bestM {
			bestM = v.metrics[s]
			best = s
		}
	}
	state := best
	outBits := 8 * len(out)
	for t := nbits - 1; t >= 0; t-- {
		full := int(v.decisions[t*convStates+state])
		if t < outBits {
			if full>>6 == 1 {
				out[t>>3] |= 1 << uint(7-t&7)
			} else {
				out[t>>3] &^= 1 << uint(7-t&7)
			}
		}
		state = full & 63
	}
}
