package lrpt

import "math"

// MSU-MR image compression is JPEG-like: 8x8 DCT blocks, the standard
// JPEG Annex K luminance quantization table scaled by a per-packet
// quality factor, and the standard Annex K luminance Huffman tables.
// DC prediction is reset at the start of every packet (14-MCU segment),
// which keeps packets independently decodable.

// Standard JPEG luminance quantization table (quality 50), row-major.
var jpegStdQuant = [64]int{
	16, 11, 10, 16, 24, 40, 51, 61,
	12, 12, 14, 19, 26, 58, 60, 55,
	14, 13, 16, 24, 40, 57, 69, 56,
	14, 17, 22, 29, 51, 87, 80, 62,
	18, 22, 37, 56, 68, 109, 103, 77,
	24, 35, 55, 64, 81, 104, 113, 92,
	49, 64, 78, 87, 103, 121, 120, 101,
	72, 92, 95, 98, 112, 100, 103, 99,
}

// Zigzag scan order: index i of the scan → raster position zigzag[i].
var jpegZigzag = [64]int{
	0, 1, 8, 16, 9, 2, 3, 10,
	17, 24, 32, 25, 18, 11, 4, 5,
	12, 19, 26, 33, 40, 48, 41, 34,
	27, 20, 13, 6, 7, 14, 21, 28,
	35, 42, 49, 56, 57, 50, 43, 36,
	29, 22, 15, 23, 30, 37, 44, 51,
	58, 59, 52, 45, 38, 31, 39, 46,
	53, 60, 61, 54, 47, 55, 62, 63,
}

// Standard Annex K luminance Huffman tables.
// bits[l] = number of codes of length l (1-16), followed by the symbol
// values in canonical order.
var jpegDCBits = [17]int{0, 0, 1, 5, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0}
var jpegDCVals = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}

var jpegACBits = [17]int{0, 0, 2, 1, 3, 3, 2, 4, 3, 5, 5, 4, 4, 0, 0, 1, 125}
var jpegACVals = []byte{
	1, 2,
	3,
	0, 4, 17,
	5, 18, 33,
	49, 65,
	6, 19, 81, 97,
	7, 34, 113,
	20, 50, 129, 145, 161,
	8, 35, 66, 177, 193,
	21, 82, 209, 240,
	36, 51, 98, 114,
	130,
	9, 10, 22, 23, 24, 25, 26, 37, 38, 39, 40, 41, 42, 52, 53, 54, 55, 56, 57,
	58, 67, 68, 69, 70, 71, 72, 73, 74, 83, 84, 85, 86, 87, 88, 89, 90, 99, 100,
	101, 102, 103, 104, 105, 106, 115, 116, 117, 118, 119, 120, 121, 122, 131,
	132, 133, 134, 135, 136, 137, 138, 146, 147, 148, 149, 150, 151, 152, 153,
	154, 162, 163, 164, 165, 166, 167, 168, 169, 170, 178, 179, 180, 181, 182,
	183, 184, 185, 186, 194, 195, 196, 197, 198, 199, 200, 201, 202, 210, 211,
	212, 213, 214, 215, 216, 217, 218, 225, 226, 227, 228, 229, 230, 231, 232,
	233, 234, 241, 242, 243, 244, 245, 246, 247, 248, 249, 250,
}

// huffTable is a canonical Huffman decoding table (JPEG style).
type huffTable struct {
	minCode [17]int32
	maxCode [17]int32 // -1 when no codes of this length
	valPtr  [17]int
	vals    []byte
}

func newHuffTable(bits [17]int, vals []byte) *huffTable {
	h := &huffTable{vals: vals}
	code := int32(0)
	k := 0
	for l := 1; l <= 16; l++ {
		if bits[l] == 0 {
			h.maxCode[l] = -1
		} else {
			h.valPtr[l] = k
			h.minCode[l] = code
			code += int32(bits[l])
			k += bits[l]
			h.maxCode[l] = code - 1
		}
		code <<= 1
	}
	return h
}

// bitReader reads MSB-first bits from a byte slice.
type bitReader struct {
	data []byte
	pos  int // bit position
	bad  bool
}

func (b *bitReader) bit() int32 {
	if b.pos >= len(b.data)*8 {
		b.bad = true
		return 0
	}
	v := int32(b.data[b.pos>>3]>>uint(7-b.pos&7)) & 1
	b.pos++
	return v
}

func (b *bitReader) bits(n int) int32 {
	var v int32
	for range n {
		v = v<<1 | b.bit()
	}
	return v
}

func (h *huffTable) decode(br *bitReader) int {
	code := br.bit()
	for l := 1; l <= 16; l++ {
		if h.maxCode[l] >= 0 && code <= h.maxCode[l] {
			return int(h.vals[h.valPtr[l]+int(code-h.minCode[l])])
		}
		code = code<<1 | br.bit()
	}
	br.bad = true
	return -1
}

// extend performs JPEG sign extension of an n-bit magnitude value.
func extend(v int32, n int) int32 {
	if n == 0 {
		return 0
	}
	if v < 1<<(n-1) {
		return v - (1<<n - 1)
	}
	return v
}

type jpegDecoder struct {
	dc     *huffTable
	ac     *huffTable
	cosLUT [8][8]float64
	lastQ  int
	quant  [64]int
}

func newJPEGDecoder() *jpegDecoder {
	j := &jpegDecoder{
		dc:    newHuffTable(jpegDCBits, jpegDCVals),
		ac:    newHuffTable(jpegACBits, jpegACVals),
		lastQ: -1,
	}
	for x := range 8 {
		for u := range 8 {
			j.cosLUT[x][u] = math.Cos(float64(2*x+1) * float64(u) * math.Pi / 16)
		}
	}
	return j
}

// quantTable computes the quantization table for quality factor q.
func (j *jpegDecoder) quantTable(q int) *[64]int {
	if q == j.lastQ {
		return &j.quant
	}
	if q < 1 {
		q = 1
	}
	if q > 100 {
		q = 100
	}
	var ratio int
	if q < 50 {
		ratio = 5000 / q
	} else {
		ratio = 200 - 2*q
	}
	for i := range 64 {
		v := (jpegStdQuant[i]*ratio/50 + 1) / 2
		if v < 1 {
			v = 1
		}
		j.quant[i] = v
	}
	j.lastQ = q
	return &j.quant
}

// decodeMCUs decodes nMCU consecutive 8x8 blocks from the bitstream and
// writes them into out as an 8-row × nMCU*8-column grayscale image.
// Returns false if the bitstream is corrupt.
func (j *jpegDecoder) decodeMCUs(data []byte, q, nMCU int, out []byte) bool {
	br := &bitReader{data: data}
	qt := j.quantTable(q)
	width := nMCU * 8
	var dcPred int32

	var coef [64]int32
	for m := range nMCU {
		for i := range coef {
			coef[i] = 0
		}

		// DC
		cat := j.dc.decode(br)
		if cat < 0 || cat > 11 {
			return false
		}
		dcPred += extend(br.bits(cat), cat)
		coef[0] = dcPred

		// AC
		for k := 1; k <= 63; {
			rs := j.ac.decode(br)
			if rs < 0 {
				return false
			}
			r, s := rs>>4, rs&0x0F
			if s == 0 {
				if r == 15 { // ZRL: run of 16 zeros
					k += 16
					continue
				}
				break // EOB
			}
			k += r
			if k > 63 {
				return false
			}
			coef[jpegZigzag[k]] = extend(br.bits(s), s)
			k++
		}
		if br.bad {
			return false
		}

		// Dequantize + IDCT
		var f [64]float64
		for i := range 64 {
			f[i] = float64(coef[i] * int32(qt[i]))
		}
		for y := range 8 {
			for x := range 8 {
				var sum float64
				for v := range 8 {
					cv := 1.0
					if v == 0 {
						cv = math.Sqrt2 / 2
					}
					var inner float64
					for u := range 8 {
						cu := 1.0
						if u == 0 {
							cu = math.Sqrt2 / 2
						}
						inner += cu * f[v*8+u] * j.cosLUT[x][u]
					}
					sum += cv * inner * j.cosLUT[y][v]
				}
				px := int(math.Round(sum/4)) + 128
				if px < 0 {
					px = 0
				} else if px > 255 {
					px = 255
				}
				out[y*width+m*8+x] = byte(px)
			}
		}
	}
	return true
}
