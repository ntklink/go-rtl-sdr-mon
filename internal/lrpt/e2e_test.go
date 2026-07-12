package lrpt

import (
	"math"
	"math/rand"
	"testing"
)

// ---- test-side JPEG encoder ----

type huffEnc struct {
	code [256]uint32
	size [256]int
}

func newHuffEnc(bits [17]int, vals []byte) *huffEnc {
	h := &huffEnc{}
	code := uint32(0)
	k := 0
	for l := 1; l <= 16; l++ {
		for range bits[l] {
			h.code[vals[k]] = code
			h.size[vals[k]] = l
			code++
			k++
		}
		code <<= 1
	}
	return h
}

type bitWriter struct {
	data []byte
	acc  uint64
	n    int
}

func (w *bitWriter) write(v uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		w.acc = w.acc<<1 | uint64(v>>uint(i)&1)
		w.n++
		if w.n == 8 {
			w.data = append(w.data, byte(w.acc))
			w.acc, w.n = 0, 0
		}
	}
}

func (w *bitWriter) flush() {
	for w.n != 0 {
		w.write(1, 1)
	}
}

func category(v int32) int {
	n := 0
	a := v
	if a < 0 {
		a = -a
	}
	for a != 0 {
		n++
		a >>= 1
	}
	return n
}

func magBits(v int32, cat int) uint32 {
	if v < 0 {
		return uint32(v + (1 << cat) - 1)
	}
	return uint32(v)
}

// encodeMCUs is the inverse of jpegDecoder.decodeMCUs for tests.
func encodeMCUs(t *testing.T, j *jpegDecoder, pixels []byte, q, nMCU int) []byte {
	t.Helper()
	dcEnc := newHuffEnc(jpegDCBits, jpegDCVals)
	acEnc := newHuffEnc(jpegACBits, jpegACVals)
	qt := j.quantTable(q)
	w := &bitWriter{}
	width := nMCU * 8
	var dcPred int32

	for m := range nMCU {
		// FDCT
		var f [64]float64
		for v := range 8 {
			for u := range 8 {
				var sum float64
				for y := range 8 {
					for x := range 8 {
						px := float64(pixels[y*width+m*8+x]) - 128
						sum += px * j.cosLUT[x][u] * j.cosLUT[y][v]
					}
				}
				cu, cv := 1.0, 1.0
				if u == 0 {
					cu = math.Sqrt2 / 2
				}
				if v == 0 {
					cv = math.Sqrt2 / 2
				}
				f[v*8+u] = sum * cu * cv / 4
			}
		}
		// Quantize in zigzag order
		var zz [64]int32
		for i := range 64 {
			zz[i] = int32(math.Round(f[jpegZigzag[i]] / float64(qt[jpegZigzag[i]])))
		}
		// DC
		diff := zz[0] - dcPred
		dcPred = zz[0]
		cat := category(diff)
		w.write(dcEnc.code[cat], dcEnc.size[cat])
		w.write(magBits(diff, cat), cat)
		// AC
		run := 0
		for k := 1; k <= 63; k++ {
			if zz[k] == 0 {
				run++
				continue
			}
			for run >= 16 {
				w.write(acEnc.code[0xF0], acEnc.size[0xF0]) // ZRL
				run -= 16
			}
			cat := category(zz[k])
			sym := byte(run<<4 | cat)
			w.write(acEnc.code[sym], acEnc.size[sym])
			w.write(magBits(zz[k], cat), cat)
			run = 0
		}
		if run > 0 {
			w.write(acEnc.code[0x00], acEnc.size[0x00]) // EOB
		}
	}
	w.flush()
	return w.data
}

// ---- packet building ----

func buildImagePacket(apid, seq int, dayMs float64, mcuIdx, q int, jpegData []byte) []byte {
	userLen := pktTimeLen + pktMCUHdrLen + len(jpegData)
	pkt := make([]byte, pktHdrLen+userLen)
	pkt[0] = 0x08 | byte(apid>>8)&0x07 // sec hdr flag + apid hi
	pkt[1] = byte(apid)
	pkt[2] = 0xC0 | byte(seq>>8)&0x3F
	pkt[3] = byte(seq)
	pkt[4] = byte((userLen - 1) >> 8)
	pkt[5] = byte(userLen - 1)
	day := int(dayMs / 86400000)
	ms := int(dayMs) % 86400000
	pkt[6] = byte(day >> 8)
	pkt[7] = byte(day)
	pkt[8] = byte(ms >> 24)
	pkt[9] = byte(ms >> 16)
	pkt[10] = byte(ms >> 8)
	pkt[11] = byte(ms)
	// pkt[12:14] = µs (zero)
	pkt[14] = byte(mcuIdx) // MCU header
	// pkt[15:17] scan header, pkt[17:19] segment header bytes 0-1
	pkt[19] = byte(q)
	copy(pkt[20:], jpegData)
	return pkt
}

// packetsToVCDUs packs a CCSDS packet stream into VCDU MPDU zones.
func packetsToVCDUs(pkts [][]byte) [][]byte {
	var stream []byte
	// byte index → true if a packet header starts here
	starts := map[int]bool{}
	for _, p := range pkts {
		starts[len(stream)] = true
		stream = append(stream, p...)
	}
	var vcdus [][]byte
	for off, ctr := 0, 0; off < len(stream); off, ctr = off+MPDUDataLen, ctr+1 {
		end := min(off+MPDUDataLen, len(stream))
		zone := make([]byte, MPDUDataLen)
		copy(zone, stream[off:end])
		hdrPtr := 0x7FF
		for i := off; i < end; i++ {
			if starts[i] {
				hdrPtr = i - off
				break
			}
		}
		vcdus = append(vcdus, makeVCDU(ctr, hdrPtr, zone))
	}
	return vcdus
}

// ---- IQ modulation ----

// modulateIQ QPSK-modulates coded bits with RRC pulse shaping at the
// given sample rate, applying a carrier offset and AWGN.
func modulateIQ(coded []byte, fs, freqOffset, noiseStd float64, rng *rand.Rand) []complex128 {
	sps := fs / SymbolRate
	nSym := len(coded) / 2
	syms := make([]complex128, nSym)
	for k := range nSym {
		i := 1 - 2*float64(coded[2*k])
		q := 1 - 2*float64(coded[2*k+1])
		syms[k] = complex(i/math.Sqrt2, q/math.Sqrt2)
	}

	nOut := int(float64(nSym+rrcSpan) * sps)
	out := make([]complex128, nOut)
	// continuous RRC pulse shaping (no timing quantization)
	half := float64(rrcSpan) / 2
	for k, s := range syms {
		center := (float64(k) + half) * sps
		lo := int(center - half*sps)
		hi := int(center + half*sps)
		for n := max(lo, 0); n <= hi && n < nOut; n++ {
			out[n] += s * complex(rrcPulse((float64(n)-center)/sps), 0)
		}
	}

	for n := range out {
		ph := 2 * math.Pi * freqOffset * float64(n) / fs
		rot := complex(math.Cos(ph), math.Sin(ph))
		out[n] = out[n]*rot + complex(rng.NormFloat64()*noiseStd, rng.NormFloat64()*noiseStd)
	}
	return out
}

// ---- tests ----

func TestJPEGRoundtrip(t *testing.T) {
	j := newJPEGDecoder()
	width := MCUPerPacket * 8
	src := make([]byte, StripHeight*width)
	for y := range StripHeight {
		for x := range width {
			src[y*width+x] = byte(64 + (x+y*8)%128)
		}
	}
	for _, q := range []int{60, 80, 95} {
		data := encodeMCUs(t, j, src, q, MCUPerPacket)
		dst := make([]byte, len(src))
		if !j.decodeMCUs(data, q, MCUPerPacket, dst) {
			t.Fatalf("q=%d: decode failed", q)
		}
		var maxErr int
		for i := range src {
			e := int(dst[i]) - int(src[i])
			if e < 0 {
				e = -e
			}
			if e > maxErr {
				maxErr = e
			}
		}
		if maxErr > 24 {
			t.Fatalf("q=%d: max pixel error %d", q, maxErr)
		}
	}
}

func TestPacketAssembly(t *testing.T) {
	j := newJPEGDecoder()
	p := newPacketParser()

	// Two strips × 3 APIDs × 14 packets covering a full line each
	apids := []int{64, 65, 66}
	type want struct {
		apid, strip, mcuIdx int
	}
	var wants []want
	var pkts [][]byte
	seq := 0
	t0 := 43200000.0 // noon
	for strip := range 2 {
		ts := t0 + float64(strip)*StripPeriodMs
		for _, apid := range apids {
			for blk := 0; blk < MCUPerLine; blk += MCUPerPacket {
				pix := make([]byte, StripHeight*MCUPerPacket*8)
				for i := range pix {
					pix[i] = byte(apid + blk + i%97)
				}
				data := encodeMCUs(t, j, pix, 85, MCUPerPacket)
				pkts = append(pkts, buildImagePacket(apid, seq, ts, blk, 85, data))
				wants = append(wants, want{apid, strip, blk})
				seq++
			}
		}
	}

	var segs []ImageSegment
	for _, v := range packetsToVCDUs(pkts) {
		segs = append(segs, p.processVCDU(v)...)
	}

	if len(segs) != len(wants) {
		t.Fatalf("got %d segments, want %d", len(segs), len(wants))
	}
	for i, w := range wants {
		s := segs[i]
		if s.APID != w.apid || s.Strip != w.strip || s.MCUIndex != w.mcuIdx {
			t.Fatalf("segment %d: got apid=%d strip=%d mcu=%d, want %+v",
				i, s.APID, s.Strip, s.MCUIndex, w)
		}
		if len(s.Pixels) != StripHeight*MCUPerPacket*8 {
			t.Fatalf("segment %d: pixel len %d", i, len(s.Pixels))
		}
	}
}

func TestFullChain(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	j := newJPEGDecoder()

	// Build image packets: 1 strip, 3 APIDs, full lines, recognizable
	// gradient content.
	var pkts [][]byte
	srcPix := map[int][]byte{}
	seq := 0
	for _, apid := range []int{64, 65, 66} {
		full := make([]byte, StripHeight*ImageWidth)
		for y := range StripHeight {
			for x := range ImageWidth {
				full[y*ImageWidth+x] = byte(40 + apid + (x/12+y*4)%160)
			}
		}
		srcPix[apid] = full
		for blk := 0; blk < MCUPerLine; blk += MCUPerPacket {
			pix := make([]byte, StripHeight*MCUPerPacket*8)
			for y := range StripHeight {
				copy(pix[y*MCUPerPacket*8:(y+1)*MCUPerPacket*8],
					full[y*ImageWidth+blk*8:y*ImageWidth+(blk+MCUPerPacket)*8])
			}
			data := encodeMCUs(t, j, pix, 85, MCUPerPacket)
			pkts = append(pkts, buildImagePacket(apid, seq, 43200000, blk, 85, data))
			seq++
		}
	}

	// VCDUs → CADUs → conv encode → QPSK IQ
	rs := newRSCodec()
	df := newDeframer()
	var stream []byte
	for _, v := range packetsToVCDUs(pkts) {
		stream = append(stream, buildCADU(t, rs, &df.pn, v)...)
	}
	// Prepend garbage so sync starts mid-stream, and append tail padding
	// so the last frame's traceback has data.
	pre := make([]byte, 512)
	rng.Read(pre)
	post := make([]byte, 512)
	rng.Read(post)
	full := append(append(pre, stream...), post...)
	coded, _ := ConvEncode(full, 0)

	fs := 257142.86                              // 1.8 MS/s ÷ 7, the real DDC output rate
	iq := modulateIQ(coded, fs, 1800, 0.18, rng) // 1.8 kHz offset + noise

	dec := NewDecoder(fs)
	var segs []ImageSegment
	for off := 0; off < len(iq); off += 8192 {
		end := min(off+8192, len(iq))
		segs = append(segs, dec.Process(iq[off:end])...)
	}

	st := dec.Stats()
	t.Logf("stats: locked=%v q=%.0f%% foff=%.0fHz framesOK=%d framesBad=%d rs=%d packets=%d apids=%v",
		st.Locked, st.SignalQ, st.FreqOffset, st.FramesOK, st.FramesBad,
		st.RSCorrect, st.Packets, st.APIDs)

	if !st.Locked {
		t.Fatal("demodulator did not lock")
	}
	if st.FramesOK < 5 {
		t.Fatalf("framesOK = %d", st.FramesOK)
	}
	wantSegs := 3 * (MCUPerLine / MCUPerPacket)
	if len(segs) < wantSegs-2 {
		t.Fatalf("decoded %d segments, want ~%d", len(segs), wantSegs)
	}

	// Verify pixel content against the source
	var maxErr int
	for _, s := range segs {
		src := srcPix[s.APID]
		if src == nil || s.Strip != 0 {
			t.Fatalf("unexpected segment %+v", s)
		}
		for y := range StripHeight {
			for x := range MCUPerPacket * 8 {
				got := int(s.Pixels[y*MCUPerPacket*8+x])
				want := int(src[y*ImageWidth+s.MCUIndex*8+x])
				e := got - want
				if e < 0 {
					e = -e
				}
				if e > maxErr {
					maxErr = e
				}
			}
		}
	}
	if maxErr > 24 {
		t.Fatalf("max pixel error %d", maxErr)
	}
	t.Logf("max pixel error: %d", maxErr)
}

func BenchmarkDecoder(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	data := make([]byte, 64*1024)
	rng.Read(data)
	coded, _ := ConvEncode(data, 0)
	fs := 257142.86
	iq := modulateIQ(coded, fs, 1000, 0.15, rng)
	dec := NewDecoder(fs)
	b.ResetTimer()
	done := 0
	for done < b.N {
		n := min(8192, b.N-done)
		off := done % (len(iq) - 8192)
		dec.Process(iq[off : off+n])
		done += n
	}
}
