package lrpt

import (
	"math/rand"
	"testing"
)

func TestPNSequence(t *testing.T) {
	d := newDeframer()
	want := []byte{0xFF, 0x48, 0x0E, 0xC0, 0x9A, 0x0D, 0x70, 0xBC}
	for i, w := range want {
		if d.pn[i] != w {
			t.Fatalf("pn[%d] = %02X, want %02X", i, d.pn[i], w)
		}
	}
}

func TestViterbiRoundtrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	data := make([]byte, 256)
	rng.Read(data)

	coded, _ := ConvEncode(data, 0)
	soft := make([]int8, len(coded))
	for i, b := range coded {
		v := int8(96)
		if b == 1 {
			v = -96
		}
		// add noise
		n := int(rng.NormFloat64() * 40)
		x := int(v) + n
		soft[i] = clampSoft(float64(x))
	}

	v := newViterbi()
	out := make([]byte, len(data))
	v.decode(soft, len(data)*8, out)

	errs := 0
	for i := range data {
		if out[i] != data[i] {
			errs++
		}
	}
	// Discount the first few bytes: without a known start state the
	// beginning of the traceback may differ.
	for i := 2; i < len(data); i++ {
		if out[i] != data[i] {
			t.Fatalf("byte %d: got %02X want %02X (total %d errs)", i, out[i], data[i], errs)
		}
	}
}

func TestRSRoundtrip(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	rs := newRSCodec()

	data := make([]byte, RSDataLen)
	rng.Read(data)
	parity := rs.encode(data)
	cw := append(append([]byte{}, data...), parity...)

	// no errors
	clean := append([]byte{}, cw...)
	if n := rs.decode(clean); n != 0 {
		t.Fatalf("clean decode = %d, want 0", n)
	}

	// up to 16 byte errors
	for nerr := 1; nerr <= 16; nerr++ {
		bad := append([]byte{}, cw...)
		positions := rng.Perm(RSBlockLen)[:nerr]
		for _, p := range positions {
			bad[p] ^= byte(1 + rng.Intn(255))
		}
		n := rs.decode(bad)
		if n != nerr {
			t.Fatalf("nerr=%d: decode returned %d", nerr, n)
		}
		for i := range cw {
			if bad[i] != cw[i] {
				t.Fatalf("nerr=%d: byte %d not corrected", nerr, i)
			}
		}
	}

	// 17 errors must be flagged uncorrectable (or at least not silently
	// miscorrected to the original)
	bad := append([]byte{}, cw...)
	for _, p := range rng.Perm(RSBlockLen)[:17] {
		bad[p] ^= byte(1 + rng.Intn(255))
	}
	if n := rs.decode(bad); n >= 0 {
		same := true
		for i := range cw {
			if bad[i] != cw[i] {
				same = false
				break
			}
		}
		if same {
			t.Fatalf("17 errors silently 'corrected' to original (n=%d)", n)
		}
	}
}

// buildCADU creates a valid randomized+RS-encoded CADU containing the
// given 892-byte VCDU.
func buildCADU(t *testing.T, rs *rsCodec, pn *[CADUDataLen]byte, vcdu []byte) []byte {
	t.Helper()
	if len(vcdu) != VCDULen {
		t.Fatalf("vcdu len %d", len(vcdu))
	}
	block := make([]byte, CADUDataLen)
	for i := range RSInterleave {
		var cw [RSBlockLen]byte
		for j := range RSDataLen {
			cw[j] = vcdu[j*RSInterleave+i]
		}
		par := rs.encode(cw[:RSDataLen])
		copy(cw[RSDataLen:], par)
		for j := range RSBlockLen {
			block[j*RSInterleave+i] = cw[j]
		}
	}
	for i := range block {
		block[i] ^= pn[i]
	}
	cadu := make([]byte, 0, CADULen)
	cadu = append(cadu, 0x1A, 0xCF, 0xFC, 0x1D)
	cadu = append(cadu, block...)
	return cadu
}

// makeVCDU builds a VCDU with the given counter whose MPDU zone is
// filled from the packet stream reader.
func makeVCDU(ctr int, hdrPtr int, zone []byte) []byte {
	v := make([]byte, VCDULen)
	v[0] = 0x40 // version/SCID
	v[2] = byte(ctr >> 16)
	v[3] = byte(ctr >> 8)
	v[4] = byte(ctr)
	v[8] = byte(hdrPtr>>8) & 0x07
	v[9] = byte(hdrPtr)
	copy(v[MPDUDataOffset:], zone)
	return v
}

func TestDeframerBitstream(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	d := newDeframer()

	// Three consecutive CADUs with random payloads
	var stream []byte
	var vcdus [][]byte
	for c := range 3 {
		vcdu := make([]byte, VCDULen)
		rng.Read(vcdu)
		vcdu[8] = 0x07 // header ptr 0x7FF (no packets, content irrelevant)
		vcdu[9] = 0xFF
		vcdu[2], vcdu[3], vcdu[4] = 0, 0, byte(c)
		vcdus = append(vcdus, vcdu)
		stream = append(stream, buildCADU(t, d.rs, &d.pn, vcdu)...)
	}

	// Convolutionally encode the whole stream continuously
	coded, _ := ConvEncode(stream, 0)

	// Modulate to soft symbols with an ambiguity transform + noise,
	// with random garbage before and after.
	for amb := range 8 {
		d.reset()
		soft := make([]int8, 0, len(coded)+8000)
		for range 2000 {
			soft = append(soft, int8(rng.Intn(200)-100))
		}
		for i := 0; i < len(coded); i += 2 {
			si := int8(90)
			if coded[i] == 1 {
				si = -90
			}
			sq := int8(90)
			if coded[i+1] == 1 {
				sq = -90
			}
			si, sq = ambTransform(amb, si, sq)
			ni := clampSoft(float64(si) + rng.NormFloat64()*25)
			nq := clampSoft(float64(sq) + rng.NormFloat64()*25)
			soft = append(soft, ni, nq)
		}
		for range 2000 {
			soft = append(soft, int8(rng.Intn(200)-100))
		}

		var got [][]byte
		// Feed in chunks to exercise buffering
		for off := 0; off < len(soft); off += 4096 {
			end := min(off+4096, len(soft))
			for _, v := range d.process(soft[off:end]) {
				got = append(got, append([]byte{}, v...))
			}
		}

		if len(got) < 2 {
			t.Fatalf("amb=%d: decoded %d VCDUs, want >= 2", amb, len(got))
		}
		// First decodable frame may be the 1st or 2nd depending on where
		// sync landed; verify content matches the originals in order.
		base := -1
		for i, v := range vcdus {
			if string(got[0]) == string(v) {
				base = i
				break
			}
		}
		if base < 0 {
			t.Fatalf("amb=%d: first VCDU doesn't match any original", amb)
		}
		for k, g := range got {
			if base+k >= len(vcdus) {
				break
			}
			if string(g) != string(vcdus[base+k]) {
				t.Fatalf("amb=%d: VCDU %d mismatch", amb, k)
			}
		}
	}
}
