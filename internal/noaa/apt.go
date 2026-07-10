package noaa

import (
	"math"
)

// biquad is a direct-form II transposed biquadratic IIR filter.
type biquad struct {
	b0, b1, b2 float64 // numerator coefficients
	a1, a2     float64 // denominator coefficients (a0 normalized to 1)
	z1, z2     float64 // state variables
}

func newBandpass(fs, f0, q float64) *biquad {
	w0 := 2 * math.Pi * f0 / fs
	alpha := math.Sin(w0) / (2 * q)
	b0 := alpha
	b1 := 0.0
	b2 := -alpha
	a0 := 1 + alpha
	a1 := -2 * math.Cos(w0)
	a2 := 1 - alpha
	return &biquad{b0: b0 / a0, b1: b1 / a0, b2: b2 / a0, a1: a1 / a0, a2: a2 / a0}
}

func newLowpass(fs, f0, q float64) *biquad {
	w0 := 2 * math.Pi * f0 / fs
	alpha := math.Sin(w0) / (2 * q)
	b0 := (1 - math.Cos(w0)) / 2
	b1 := 1 - math.Cos(w0)
	b2 := (1 - math.Cos(w0)) / 2
	a0 := 1 + alpha
	a1 := -2 * math.Cos(w0)
	a2 := 1 - alpha
	return &biquad{b0: b0 / a0, b1: b1 / a0, b2: b2 / a0, a1: a1 / a0, a2: a2 / a0}
}

func (b *biquad) process(x float64) float64 {
	y := b.b0*x + b.z1
	b.z1 = b.b1*x - b.a1*y + b.z2
	b.z2 = b.b2*x - b.a2*y
	return y
}

// APTDecoder processes FM-demodulated audio and decodes APT images from
// NOAA polar-orbiting weather satellites.
//
// The APT signal uses a 2.4 kHz subcarrier that is AM-modulated with pixel
// brightness data. Each line is 0.5 seconds (2080 pixels at 4160 Hz).
// Sync A is 7 cycles at 1040 Hz; sync B is 7 cycles at 832 Hz.
//
// Decoding chain:
//  1. Bandpass around 2.4 kHz (wide enough to pass the full AM sidebands,
//     320–4480 Hz) → rectify → cascaded lowpass envelope
//  2. Linear-interpolated resampling to the 4160 Hz pixel rate
//  3. Per-line sync-A correlation to keep rows aligned (this is the key
//     fix for "garbled" images: doppler shift and non-integer sample
//     ratios cause progressive row skew if sync is only acquired once).
type APTDecoder struct {
	sampleRate float64 // input audio rate (typically 48000)

	// AM demod chain
	bpSubcarrier *biquad // bandpass centered at 2.4 kHz, wide enough for AM sidebands
	lpEnvelope1  *biquad // first lowpass stage for envelope extraction
	lpEnvelope2  *biquad // second lowpass stage (steeper rolloff)

	// Resampling: sampleRate → PixelRate (4160 Hz) via linear interpolation
	samplesPerPixel float64 // sampleRate / PixelRate
	phase           float64 // accumulated fractional phase
	prevSample      float64 // previous filtered sample (for linear interp)

	// Pixel stream buffer (continuously growing; pruned after line extraction)
	pixelStream []float64

	// Per-line sync tracking
	syncPattern []float64 // 32-sample sync A reference (aptdec-style)
	lineStart   int       // index into pixelStream where the current line starts
	syncLocked  bool      // whether we have ever acquired sync

	// Doppler / sample-rate tracking: the measured offset of each sync
	// pulse from its expected position tells us how much the effective
	// samples-per-pixel differs from the nominal value.  A slow EMA
	// adjusts the resampler to compensate.
	sppCorrection float64

	// Signal level: EMA of the 2.4 kHz subcarrier envelope RMS, used as a
	// diagnostic to tell "no signal" (antenna/frequency problem) from
	// "bad decoding".
	signalLevel float64

	// Consecutive sync hits before locking (reduces false positives on noise)
	syncStreak int

	// Image storage
	lines    [][]byte
	maxLines int
	ringIdx  int

	// Brightness reference (slow peak tracker for consistent scaling)
	peakEnv float64

	// Statistics
	lineCount    int
	linesDecoded int
	syncFound    int
}

// NewAPTDecoder creates a new APT decoder for the given audio sample rate.
func NewAPTDecoder(sampleRate float64) *APTDecoder {
	d := &APTDecoder{
		sampleRate:      sampleRate,
		samplesPerPixel: sampleRate / PixelRate,
		maxLines:        2000,
	}

	// Bandpass: center 2.4 kHz, Q=0.6 → bandwidth ~4000 Hz, covering the
	// full AM sidebands (2400 ± 2080 Hz = 320–4480 Hz).  The previous
	// Q=3 (800 Hz bandwidth) clipped most of the image content.
	d.bpSubcarrier = newBandpass(sampleRate, 2400, 0.6)

	// Cascaded lowpass at 2080 Hz (pixel Nyquist) for envelope extraction.
	// Two stages give a steeper rolloff than a single biquad.
	d.lpEnvelope1 = newLowpass(sampleRate, 2080, 0.707)
	d.lpEnvelope2 = newLowpass(sampleRate, 2080, 0.707)

	// Sync A reference pattern (32 samples at the pixel rate).
	// This mirrors aptdec's sync_pattern: 7 cycles of a 1040 Hz pulse
	// train (2 high / 2 low per cycle = 28 samples) with 4 transition
	// samples.  Values are asymmetric (-14/+18) to match the DC offset of
	// the real APT sync waveform.
	d.syncPattern = []float64{
		-14, -14, -14, 18, 18, -14, -14, 18, 18, -14, -14, 18, 18, -14, -14, 18,
		18, -14, -14, 18, 18, -14, -14, 18, 18, -14, -14, 18, 18, -14, -14, -14,
	}

	return d
}

// Process feeds audio samples (float64, 48 kHz) to the decoder.
func (d *APTDecoder) Process(samples []float64) []APTLine {
	var completed []APTLine

	for _, s := range samples {
		// 1. Bandpass filter around 2.4 kHz subcarrier
		filtered := d.bpSubcarrier.process(s)

		// 2. Rectify (envelope detection)
		rect := math.Abs(filtered)

		// 3. Cascaded lowpass to get smooth envelope
		env := d.lpEnvelope1.process(rect)
		env = d.lpEnvelope2.process(env)

		// Track signal level (EMA of envelope) for diagnostics.
		d.signalLevel = d.signalLevel*0.999 + env*0.001

		// 4. Resample to pixel rate (4160 Hz) via linear interpolation.
		d.phase += 1.0
		effectiveSPP := d.samplesPerPixel + d.sppCorrection
		if effectiveSPP < 1 {
			effectiveSPP = 1
		}
		for d.phase >= effectiveSPP {
			frac := d.phase / effectiveSPP
			if frac > 1 {
				frac = 1
			}
			pixel := d.prevSample + frac*(env-d.prevSample)
			d.pixelStream = append(d.pixelStream, pixel)
			d.phase -= effectiveSPP
		}
		d.prevSample = env

		// 5. Try to extract complete lines
		completed = append(completed, d.tryExtractLines()...)
	}

	return completed
}

// tryExtractLines checks if enough pixels have accumulated to extract one or
// more complete lines, performing per-line sync-A correlation to keep rows
// aligned.
func (d *APTDecoder) tryExtractLines() []APTLine {
	var completed []APTLine
	syncLen := len(d.syncPattern)

	for {
		// We need at least one full line past the current start position
		// plus the sync search window.
		searchRange := 16
		need := d.lineStart + LinePixels + searchRange + syncLen
		if len(d.pixelStream) < need {
			break
		}

		// Search for sync A around the expected position (one line after
		// the current start).  This per-line re-alignment compensates for
		// doppler shift and non-integer sample ratios that cause
		// progressive row skew.
		bestPos := d.lineStart + LinePixels
		bestScore := d.syncScore(bestPos)

		for offset := -searchRange; offset <= searchRange; offset++ {
			if offset == 0 {
				continue
			}
			pos := d.lineStart + LinePixels + offset
			if pos < 0 || pos+syncLen > len(d.pixelStream) {
				continue
			}
			score := d.syncScore(pos)
			if score > bestScore {
				bestScore = score
				bestPos = pos
			}
		}

		if !d.syncLocked {
			// Before first lock, require a strong sync hit.  The
			// threshold of 0.55 with a 32-sample pattern over a ±16
			// search window (33 positions) keeps the false-positive
			// rate negligible (≈0.3% per line for pure noise, since
			// 1/√32 ≈ 0.18 is the noise correlation std-dev).
			// Additionally require 2 consecutive hits before locking
			// to further suppress noise-only "snow" images.
			if bestScore < 0.55 {
				d.syncStreak = 0
				d.lineStart++
				if d.lineStart > len(d.pixelStream)-LinePixels-syncLen {
					d.pruneStream()
				}
				continue
			}
			d.syncStreak++
			if d.syncStreak < 2 {
				// First hit: advance to the candidate position but
				// don't lock yet — wait for a second confirmation.
				d.lineStart = bestPos
				d.pruneStream()
				continue
			}
			d.syncLocked = true
			d.syncFound++
		} else {
			// Already locked: a very weak score means sync was lost
			// (e.g. a fade).  Fall back to nominal spacing.
			if bestScore < 0.15 {
				bestPos = d.lineStart + LinePixels
			} else {
				d.syncFound++
			}
		}

		// Track doppler: the difference between the found sync position
		// and the expected position tells us the sample-rate error.
		// A slow EMA adjusts the resampler to compensate.
		offset := bestPos - (d.lineStart + LinePixels)
		d.sppCorrection = d.sppCorrection*0.95 + float64(offset)*0.0001*d.samplesPerPixel

		// Extract one line starting at lineStart.
		line := d.finalizeLine(d.lineStart)
		completed = append(completed, line)

		// Advance to the sync position for the next line.
		d.lineStart = bestPos
		d.pruneStream()
	}

	return completed
}

// syncScore computes the normalized correlation between the sync pattern and
// the pixel stream at the given position.
func (d *APTDecoder) syncScore(pos int) float64 {
	syncLen := len(d.syncPattern)
	if pos < 0 || pos+syncLen > len(d.pixelStream) {
		return 0
	}

	var refMean float64
	for _, v := range d.syncPattern {
		refMean += v
	}
	refMean /= float64(syncLen)

	var winMean float64
	for i := range syncLen {
		winMean += d.pixelStream[pos+i]
	}
	winMean /= float64(syncLen)

	var corr, refNorm, winNorm float64
	for i := 0; i < syncLen; i++ {
		r := d.syncPattern[i] - refMean
		w := d.pixelStream[pos+i] - winMean
		corr += r * w
		refNorm += r * r
		winNorm += w * w
	}

	if refNorm <= 0 || winNorm <= 0 {
		return 0
	}
	return corr / math.Sqrt(refNorm*winNorm)
}

// finalizeLine converts LinePixels worth of pixel data starting at the given
// index into a byte line, updates the brightness reference, and stores the
// line in the ring buffer.
func (d *APTDecoder) finalizeLine(start int) APTLine {
	line := make([]byte, LinePixels)

	var lineMax float64
	for i := 0; i < LinePixels; i++ {
		if start+i < len(d.pixelStream) {
			v := d.pixelStream[start+i]
			if v > lineMax {
				lineMax = v
			}
		}
	}
	if lineMax > d.peakEnv {
		d.peakEnv = lineMax
	} else {
		d.peakEnv = d.peakEnv*0.9 + lineMax*0.1
	}
	peak := d.peakEnv
	if peak < 1e-6 {
		peak = 1
	}

	for i := range LinePixels {
		var v float64
		if start+i < len(d.pixelStream) {
			v = d.pixelStream[start+i] / peak
		}
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		line[i] = byte(v * 255)
	}

	aptLine := APTLine{
		LineNum: d.lineCount,
		Pixels:  line,
	}
	d.lineCount++
	d.linesDecoded++

	if len(d.lines) < d.maxLines {
		d.lines = append(d.lines, line)
	} else {
		d.lines[d.ringIdx] = line
		d.ringIdx = (d.ringIdx + 1) % d.maxLines
	}

	return aptLine
}

// pruneStream removes already-processed pixels from the front of the stream
// to bound memory usage.  lineStart is adjusted accordingly.
func (d *APTDecoder) pruneStream() {
	if d.lineStart <= 0 {
		return
	}
	margin := 64
	if d.lineStart < margin {
		return
	}
	cut := d.lineStart - margin
	if cut <= 0 {
		return
	}
	d.pixelStream = d.pixelStream[cut:]
	d.lineStart -= cut
}

// Stats returns decoder statistics: decoded line count, sync detection
// count, and the current 2.4 kHz subcarrier signal level (0–1 scale, EMA of
// the envelope amplitude).  A near-zero signalLevel means no satellite
// signal is being received (antenna/frequency/timing issue), while a high
// signalLevel with sync=0 suggests a decoder problem.
func (d *APTDecoder) Stats() (linesDecoded, syncFound int, signalLevel float64) {
	return d.linesDecoded, d.syncFound, d.signalLevel
}

// GetImage returns the accumulated image as a 2D byte slice.
func (d *APTDecoder) GetImage() [][]byte {
	return d.lines
}

// Reset clears the decoder state and image buffer.
func (d *APTDecoder) Reset() {
	d.pixelStream = d.pixelStream[:0]
	d.lineStart = 0
	d.syncLocked = false
	d.lineCount = 0
	d.linesDecoded = 0
	d.syncFound = 0
	d.sppCorrection = 0
	d.signalLevel = 0
	d.syncStreak = 0
	d.phase = 0
	d.prevSample = 0
	d.peakEnv = 0
	d.lines = d.lines[:0]
	d.ringIdx = 0
	d.bpSubcarrier.z1 = 0
	d.bpSubcarrier.z2 = 0
	d.lpEnvelope1.z1 = 0
	d.lpEnvelope1.z2 = 0
	d.lpEnvelope2.z1 = 0
	d.lpEnvelope2.z2 = 0
}
