package adsb

import (
	"math"
)

// Decoder takes raw IQ samples and detects ADS-B messages.
// It performs AM demodulation, preamble detection, Manchester decoding,
// and CRC verification.
type Decoder struct {
	sampleRate    float64 // Hz
	samplesPerBit float64 // sampleRate / 1e6

	// Magnitude buffer
	magBuf []float64

	// Statistics
	messagesDetected int
	messagesValid    int
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

	// Compute magnitudes (AM demodulation)
	mags := make([]float64, len(samples))
	for i, s := range samples {
		mags[i] = math.Sqrt(real(s)*real(s) + imag(s)*imag(s))
	}

	// Normalize
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

	spb := int(d.samplesPerBit) // samples per bit (should be 2 at 2 MHz)
	if spb < 1 {
		spb = 1
	}

	// Preamble pattern: at 2 MHz, 2 samples per μs
	// Preamble: high at 0μs, low at 1μs, high at 2μs, low at 3μs, low at 4μs,
	//           high at 5μs, low at 6μs, high at 7μs
	// At 2 samples/μs: high at [0,1], low at [2,3], high at [4,5], low at [6,7],
	//                  low at [8,9], high at [10,11], low at [12,13], high at [14,15]
	// Then 112 bits × 2 samples = 224 samples

	// Minimum message length: 16 (preamble) + 112*spb (message) = 16 + 224 = 240
	preambleLen := 8 * spb // 8 μs
	msgLen := 112 * spb
	totalLen := preambleLen + msgLen

	if len(mags) < totalLen {
		// Buffer remaining samples for next block
		d.magBuf = append(d.magBuf, mags...)
		if len(d.magBuf) > totalLen*2 {
			// Prevent unbounded growth
			d.magBuf = d.magBuf[len(d.magBuf)-totalLen:]
		}
		return nil
	}

	// Use buffered samples if available
	if len(d.magBuf) > 0 {
		mags = append(d.magBuf, mags...)
		d.magBuf = nil
		if len(mags) < totalLen {
			d.magBuf = mags
			return nil
		}
	}

	// Scan for preambles
	i := 0
	for i < len(mags)-totalLen {
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
						messages = append(messages, decoded)
					}
				} else {
					// Try single-bit error correction
					fixed, ok := fixSingleBitError(msg)
					if ok {
						d.messagesValid++
						decoded := decodeMessage(fixed)
						if decoded != nil && (decoded.DF == 17 || decoded.DF == 18) {
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

	// Save remaining samples for next block
	if i < len(mags) {
		d.magBuf = mags[i:]
		if len(d.magBuf) > totalLen*2 {
			d.magBuf = d.magBuf[len(d.magBuf)-totalLen:]
		}
	}

	return messages
}

// detectPreamble checks if the samples at position `pos` match the ADS-B
// preamble pattern.
func (d *Decoder) detectPreamble(mags []float64, pos, spb int) bool {
	// Preamble pulse positions (in μs from start): 0, 2, 5, 7
	// At spb samples per μs
	highIdx := []int{0, 2 * spb, 5 * spb, 7 * spb}
	lowIdx := []int{1 * spb, 3 * spb, 4 * spb, 6 * spb}

	// Check we have enough data
	if pos+8*spb >= len(mags) {
		return false
	}

	// Calculate threshold: average of high pulses
	var highSum float64
	for _, idx := range highIdx {
		highSum += mags[pos+idx]
	}
	highAvg := highSum / 4

	// Calculate average of low positions
	var lowSum float64
	for _, idx := range lowIdx {
		lowSum += mags[pos+idx]
	}
	lowAvg := lowSum / 4

	// High pulses must be significantly higher than low positions
	if highAvg < lowAvg*2 || highAvg < 0.5 {
		return false
	}

	// Each high pulse should be above threshold
	threshold := highAvg * 0.6
	for _, idx := range highIdx {
		if mags[pos+idx] < threshold {
			return false
		}
	}

	// Each low position should be below threshold
	for _, idx := range lowIdx {
		if mags[pos+idx] > threshold {
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

// Stats returns decoder statistics.
func (d *Decoder) Stats() (detected, valid int) {
	return d.messagesDetected, d.messagesValid
}
