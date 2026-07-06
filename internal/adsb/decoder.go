package adsb

import (
	"log"
	"math"
)

// Decoder takes raw IQ samples and detects ADS-B messages.
// It performs DC offset removal, AM demodulation, preamble detection,
// Manchester decoding, and CRC verification.
type Decoder struct {
	sampleRate    float64 // Hz
	samplesPerBit float64 // sampleRate / 1e6

	// Magnitude buffer (raw, un-normalized) for cross-block detection
	magBuf []float64

	// Statistics
	messagesDetected int // preamble detected + Manchester decoded
	messagesValid    int // CRC passed (any DF)
	messagesAccepted int // CRC passed + DF 17/18 (ADS-B)
}

// NewDecoder creates a new ADS-B decoder for the given sample rate.
// ADS-B uses 1 Mbit/s Manchester encoding. A sample rate of 2+ MHz is recommended.
func NewDecoder(sampleRate float64) *Decoder {
	return &Decoder{
		sampleRate:    sampleRate,
		samplesPerBit: sampleRate / 1e6,
	}
}

// Process takes a block of complex128 IQ samples and returns any decoded
// ADS-B messages found in this block.
func (d *Decoder) Process(samples []complex128) []*Message {
	var messages []*Message

	if len(samples) == 0 {
		return nil
	}

	// 1. Remove DC offset (RTL-SDR has a significant DC spike at center freq)
	var sumI, sumQ float64
	for _, s := range samples {
		sumI += real(s)
		sumQ += imag(s)
	}
	n := float64(len(samples))
	meanI := sumI / n
	meanQ := sumQ / n

	// 2. Compute magnitudes after DC removal
	mags := make([]float64, len(samples))
	for i, s := range samples {
		di := real(s) - meanI
		dq := imag(s) - meanQ
		mags[i] = math.Sqrt(di*di + dq*dq)
	}

	spb := int(d.samplesPerBit) // samples per bit (should be 2 at 2 MHz)
	if spb < 2 {
		log.Printf("ADS-B: sample rate too low for reliable decoding (spb=%d, sampleRate=%.0f Hz, need >= 2 MHz)", spb, d.sampleRate)
		return nil
	}

	// Minimum message length: 16 (preamble) + 112*spb (message) = 16 + 224 = 240
	preambleLen := 8 * spb // 8 μs
	msgLen := 112 * spb
	totalLen := preambleLen + msgLen

	// 3. Prepend buffered raw magnitudes (from previous block) for cross-block detection
	if len(d.magBuf) > 0 {
		mags = append(d.magBuf, mags...)
		d.magBuf = nil
	}

	if len(mags) < totalLen {
		// Not enough data; save for next block
		d.magBuf = mags
		if len(d.magBuf) > totalLen*2 {
			d.magBuf = d.magBuf[len(d.magBuf)-totalLen:]
		}
		return nil
	}

	// 4. Normalize the combined array by its average
	var sum float64
	for _, m := range mags {
		sum += m
	}
	avg := sum / float64(len(mags))
	if avg < 1e-10 {
		return nil
	}
	for i := range mags {
		mags[i] /= avg
	}

	// 5. Scan for preambles
	i := 0
	for i+totalLen <= len(mags) {
		if d.detectPreamble(mags, i, spb) {
			// Decode the 112-bit message
			msg := d.decodeBits(mags, i+preambleLen, spb)
			if msg != nil {
				d.messagesDetected++
				// Verify CRC
				if checkCRC(msg) {
					d.messagesValid++
					decoded := decodeMessage(msg)
					if decoded != nil && (decoded.DF == 17 || decoded.DF == 18) {
						d.messagesAccepted++
						messages = append(messages, decoded)
					}
				} else {
					// Try single-bit error correction
					fixed, ok := fixSingleBitError(msg)
					if ok {
						d.messagesValid++
						decoded := decodeMessage(fixed)
						if decoded != nil && (decoded.DF == 17 || decoded.DF == 18) {
							d.messagesAccepted++
							messages = append(messages, decoded)
						}
					}
				}
			}
			// Skip past this message
			i += totalLen
		} else {
			i++
		}
	}

	// 6. Save remaining raw magnitudes for next block
	//    We must save un-normalized values, so re-multiply by avg
	if i < len(mags) {
		remaining := make([]float64, len(mags)-i)
		for j := range remaining {
			remaining[j] = mags[i+j] * avg
		}
		d.magBuf = remaining
		if len(d.magBuf) > totalLen*2 {
			d.magBuf = d.magBuf[len(d.magBuf)-totalLen:]
		}
	}

	return messages
}

// detectPreamble checks if the samples at position `pos` match the ADS-B
// preamble pattern.
// The ADS-B preamble consists of 4 high pulses at 0, 2, 5, 7 μs and
// 4 low gaps at 1, 3, 4, 6 μs. Each position is averaged over spb samples
// to reduce noise sensitivity.
func (d *Decoder) detectPreamble(mags []float64, pos, spb int) bool {
	// Check we have enough data for 8 μs of preamble
	if pos+8*spb > len(mags) {
		return false
	}

	// Compute average magnitude at each μs position (0-7 μs)
	var usMags [8]float64
	for us := 0; us < 8; us++ {
		var s float64
		for j := 0; j < spb; j++ {
			s += mags[pos+us*spb+j]
		}
		usMags[us] = s / float64(spb)
	}

	// Preamble: high at 0, 2, 5, 7 μs; low at 1, 3, 4, 6 μs
	highVals := [4]float64{usMags[0], usMags[2], usMags[5], usMags[7]}
	lowVals := [4]float64{usMags[1], usMags[3], usMags[4], usMags[6]}

	var highSum, lowSum float64
	for _, v := range highVals {
		highSum += v
	}
	for _, v := range lowVals {
		lowSum += v
	}
	highAvg := highSum / 4
	lowAvg := lowSum / 4

	// High pulses must be at least 2x the low gaps (distinct on/off pattern)
	// and at least 2x the block average (1.0 after normalization) to avoid
	// matching noise fluctuations
	if highAvg < lowAvg*2 || highAvg < 2.0 {
		return false
	}

	// Each high pulse must be above 70% of highAvg (consistency check)
	threshold := highAvg * 0.7
	for _, v := range highVals {
		if v < threshold {
			return false
		}
	}

	// Each low gap must be below 70% of highAvg (consistency check)
	for _, v := range lowVals {
		if v > threshold {
			return false
		}
	}

	return true
}

// decodeBits performs Manchester decoding of the 112-bit message.
func (d *Decoder) decodeBits(mags []float64, start, spb int) []byte {
	// Each bit is spb samples wide
	// Manchester: bit=1 → first half high, second half low
	//             bit=0 → first half low, second half high

	msg := make([]byte, 14) // 112 bits = 14 bytes
	bitIdx := 0

	for bit := 0; bit < 112; bit++ {
		pos := start + bit*spb
		if pos+spb > len(mags) {
			return nil
		}

		// Sum first half and second half
		half := spb / 2
		if half < 1 {
			half = 1
		}

		var firstHalf, secondHalf float64
		for j := 0; j < half; j++ {
			firstHalf += mags[pos+j]
		}
		firstHalf /= float64(half)

		for j := half; j < spb; j++ {
			secondHalf += mags[pos+j]
		}
		secondHalf /= float64(spb - half)

		// Decode bit
		if firstHalf > secondHalf {
			// Bit = 1
			msg[bitIdx/8] |= 1 << (7 - bitIdx%8)
		}
		// Bit = 0 is already zero

		bitIdx++
	}

	return msg
}

// Stats returns decoder statistics: (detected, valid, accepted).
// detected = preambles found and Manchester decoded
// valid = CRC passed (any downlink format)
// accepted = CRC passed and DF=17/18 (ADS-B extended squitter)
func (d *Decoder) Stats() (detected, valid, accepted int) {
	return d.messagesDetected, d.messagesValid, d.messagesAccepted
}
