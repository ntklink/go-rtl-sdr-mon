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
//
// The Mode S preamble consists of 4 impulses of 0.5 μs each at:
//
//	0.0-0.5 μs, 1.0-1.5 μs, 3.5-4.0 μs, 4.5-5.0 μs
//
// At 2 MHz (spb=2, 1 sample = 0.5 μs) this maps to sample indices 0, 2, 7, 9.
// This is the standard dump1090 algorithm: fast rejection via pairwise
// sample comparisons, then a level check on the inter-pulse gaps and
// post-preamble quiet region.
func (d *Decoder) detectPreamble(mags []float64, pos, spb int) bool {
	// This algorithm is designed for spb == 2 (2 MHz).
	// For other rates, fall back to the generic μs-level detector.
	if spb != 2 {
		return d.detectPreambleGeneric(mags, pos, spb)
	}

	// Need 15 samples: 10 for the preamble pattern + 5 for the
	// post-preamble quiet-zone check.
	if pos+15 > len(mags) {
		return false
	}

	m := mags[pos:]

	// --- Fast rejection: relations between the first 10 samples ---
	// The preamble pattern at 2 MHz is:
	//   sample 0: HIGH (pulse 1)
	//   sample 1: low
	//   sample 2: HIGH (pulse 2)
	//   sample 3: low
	//   sample 4: low
	//   sample 5: low
	//   sample 6: low
	//   sample 7: HIGH (pulse 3)
	//   sample 8: low
	//   sample 9: HIGH (pulse 4)
	if !(m[0] > m[1] &&
		m[1] < m[2] &&
		m[0] > m[2] &&
		m[1] < m[0] &&
		m[2] > m[3] &&
		m[3] < m[0] &&
		m[4] < m[0] &&
		m[5] < m[0] &&
		m[6] < m[0] &&
		m[7] > m[8] &&
		m[8] < m[9] &&
		m[9] > m[6]) {
		return false
	}

	// --- Level check: average of the 4 high pulses ---
	// Dividing by 6 (not 4) makes the threshold ~2/3 of the true average,
	// which is more lenient and matches dump1090 behavior.
	high := (m[0] + m[2] + m[7] + m[9]) / 6

	// Samples between pulses (positions 4, 5) must be well below the
	// pulse level. We don't test positions too near to the high pulses
	// because phase offset can spread energy into adjacent samples.
	if m[4] >= high || m[5] >= high {
		return false
	}

	// Samples 11-14 (the quiet zone between preamble and data) must be low.
	if m[11] >= high || m[12] >= high || m[13] >= high || m[14] >= high {
		return false
	}

	return true
}

// detectPreambleGeneric is the fallback preamble detector for sample rates
// other than 2 MHz. It uses μs-level averaging and the (incorrect for 2 MHz
// but acceptable for higher rates) 0,2,5,7 μs pulse template.
func (d *Decoder) detectPreambleGeneric(mags []float64, pos, spb int) bool {
	if pos+8*spb > len(mags) {
		return false
	}

	var usMags [8]float64
	for us := range 8 {
		var s float64
		for j := range spb {
			s += mags[pos+us*spb+j]
		}
		usMags[us] = s / float64(spb)
	}

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

	if highAvg < lowAvg*2 || highAvg < 2.0 {
		return false
	}

	threshold := highAvg * 0.7
	for _, v := range highVals {
		if v < threshold {
			return false
		}
	}
	for _, v := range lowVals {
		if v > threshold {
			return false
		}
	}

	return true
}

// decodeBits performs Manchester decoding of the 112-bit message.
// When the two half-bit magnitudes are too close (within a small delta),
// the bit value from the previous position is reused. This "weak signal"
// handling (inspired by dump1090) reduces random bit flips caused by noise
// on marginal signals and significantly improves the CRC pass rate.
func (d *Decoder) decodeBits(mags []float64, start, spb int) []byte {
	msg := make([]byte, 14) // 112 bits = 14 bytes
	bitIdx := 0
	prevBit := 0 // used for weak-signal carry-forward

	// Weak-signal threshold: if |firstHalf - secondHalf| is below this
	// fraction of their average, the bit is considered ambiguous and we
	// reuse the previous bit's value. The magnitude array is normalized
	// to average ≈ 1.0, so typical signal values are O(1).
	const weakThreshold = 0.1

	for bit := range 112 {
		pos := start + bit*spb
		if pos+spb > len(mags) {
			return nil
		}

		half := max(spb/2, 1)

		var firstHalf, secondHalf float64
		for j := range half {
			firstHalf += mags[pos+j]
		}
		firstHalf /= float64(half)

		for j := half; j < spb; j++ {
			secondHalf += mags[pos+j]
		}
		secondHalf /= float64(spb - half)

		delta := firstHalf - secondHalf
		if delta < 0 {
			delta = -delta
		}
		avg := (firstHalf + secondHalf) / 2

		// Determine bit value
		var bitVal int
		if avg > 0 && delta < avg*weakThreshold {
			// Ambiguous: reuse previous bit (reduces noise-induced errors)
			bitVal = prevBit
		} else if firstHalf > secondHalf {
			bitVal = 1
		} else {
			bitVal = 0
		}

		if bitVal == 1 {
			msg[bitIdx/8] |= 1 << (7 - bitIdx%8)
		}
		prevBit = bitVal

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
