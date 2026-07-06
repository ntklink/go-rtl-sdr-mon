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
type APTDecoder struct {
	sampleRate float64 // input audio rate (typically 48000)

	// AM demod chain: bandpass 2.4 kHz → rectify → lowpass
	bpSubcarrier *biquad // bandpass centered at 2.4 kHz
	lpEnvelope   *biquad // lowpass for envelope extraction

	// Decimation: sampleRate → PixelRate (4160 Hz)
	samplesPerPixel float64 // sampleRate / PixelRate
	decimPhase      float64 // accumulated phase for decimation
	decimAccum      float64 // sample accumulator for current pixel
	decimCount      int     // samples accumulated for current pixel

	// Line assembly
	pixelBuf  []float64 // pixels for the current line (up to LinePixels)
	lineCount int       // total lines processed (for stats)

	// Sync detection
	syncRefA    []float64 // reference sync A pattern (normalized)
	syncFound   int       // number of sync frames detected
	syncLocked  bool      // whether we have sync lock
	lineOffset  int       // pixel offset within current line
	syncQuality float64   // last sync correlation quality (0..1)

	// Image storage
	lines    [][]byte // decoded lines (each 2080 bytes)
	maxLines int      // maximum lines to store

	// Statistics
	linesDecoded int
}

// NewAPTDecoder creates a new APT decoder for the given audio sample rate.
func NewAPTDecoder(sampleRate float64) *APTDecoder {
	d := &APTDecoder{
		sampleRate:      sampleRate,
		samplesPerPixel: sampleRate / PixelRate,
		pixelBuf:        make([]float64, 0, LinePixels),
		maxLines:        2000, // store up to 2000 lines (~16 minutes)
	}

	// Bandpass: center 2.4 kHz, Q=3 (bandwidth ~800 Hz)
	d.bpSubcarrier = newBandpass(sampleRate, 2400, 3.0)

	// Lowpass for envelope: 2.1 kHz cutoff (just below pixel Nyquist)
	d.lpEnvelope = newLowpass(sampleRate, 2100, 0.707)

	// Generate sync A reference: 7 cycles at 1040 Hz, sampled at PixelRate
	// At 4160 Hz, 1040 Hz = 4 samples per cycle, 7 cycles = 28 samples
	syncLen := 28
	d.syncRefA = make([]float64, syncLen)
	for i := 0; i < syncLen; i++ {
		// Square wave: high for first half of each cycle, low for second half
		cyclePos := i % 4 // 4 samples per cycle at 1040 Hz / 4160 Hz
		if cyclePos < 2 {
			d.syncRefA[i] = 1.0
		} else {
			d.syncRefA[i] = -1.0
		}
	}

	return d
}

// Process feeds audio samples (float64, 48 kHz) to the decoder.
// Returns any completed APT lines.
func (d *APTDecoder) Process(samples []float64) []APTLine {
	var completed []APTLine

	for _, s := range samples {
		// 1. Bandpass filter around 2.4 kHz subcarrier
		filtered := d.bpSubcarrier.process(s)

		// 2. Rectify (envelope detection)
		rect := math.Abs(filtered)

		// 3. Lowpass to get smooth envelope
		env := d.lpEnvelope.process(rect)

		// 4. Decimate to pixel rate (4160 Hz)
		d.decimAccum += env
		d.decimCount++
		d.decimPhase += 1.0

		if d.decimPhase >= d.samplesPerPixel {
			pixel := d.decimAccum / float64(d.decimCount)
			d.decimAccum = 0
			d.decimCount = 0
			d.decimPhase -= d.samplesPerPixel

			completed = append(completed, d.processPixel(pixel)...)
		}
	}

	return completed
}

// processPixel handles one decoded pixel value.
func (d *APTDecoder) processPixel(pixel float64) []APTLine {
	var completed []APTLine

	d.pixelBuf = append(d.pixelBuf, pixel)
	d.lineOffset++

	// Try sync detection when we have enough samples
	if !d.syncLocked && len(d.pixelBuf) >= 64 {
		d.trySync()
	}

	// If we have a full line, finalize it
	if d.syncLocked && d.lineOffset >= LinePixels {
		if line, ok := d.finalizeLine(); ok {
			completed = append(completed, line)
		}
	}

	// If not locked and buffer is too long, flush a line anyway (best effort)
	if !d.syncLocked && len(d.pixelBuf) >= LinePixels*2 {
		// Reset and try again
		d.pixelBuf = d.pixelBuf[len(d.pixelBuf)-LinePixels:]
		d.lineOffset = LinePixels
		if line, ok := d.finalizeLine(); ok {
			completed = append(completed, line)
		}
	}

	return completed
}

// trySync attempts to find the sync A pattern in the pixel buffer.
// If found, it realigns the buffer so that sync A starts at pixel 0.
func (d *APTDecoder) trySync() {
	buf := d.pixelBuf
	refLen := len(d.syncRefA)
	if len(buf) < refLen+10 {
		return
	}

	// Search for sync A in the first portion of the buffer
	bestCorr := -1e9
	bestPos := -1

	// Only search in a reasonable window to avoid excessive computation
	searchEnd := len(buf) - refLen
	if searchEnd > 100 {
		searchEnd = 100
	}

	// Normalize the reference (zero mean)
	var refMean float64
	for _, v := range d.syncRefA {
		refMean += v
	}
	refMean /= float64(refLen)

	for pos := 0; pos < searchEnd; pos++ {
		// Compute correlation at this position
		var corr, winMean float64
		for i := 0; i < refLen; i++ {
			winMean += buf[pos+i]
		}
		winMean /= float64(refLen)

		for i := 0; i < refLen; i++ {
			corr += (d.syncRefA[i] - refMean) * (buf[pos+i] - winMean)
		}

		if corr > bestCorr {
			bestCorr = corr
			bestPos = pos
		}
	}

	// Compute quality metric (normalized correlation)
	var refNorm, winNorm float64
	for i := 0; i < refLen; i++ {
		refNorm += (d.syncRefA[i] - refMean) * (d.syncRefA[i] - refMean)
	}
	for i := 0; i < refLen; i++ {
		dv := buf[bestPos+i] - refMean // approx window mean
		winNorm += dv * dv
	}
	if refNorm > 0 && winNorm > 0 {
		d.syncQuality = bestCorr / math.Sqrt(refNorm*winNorm)
	} else {
		d.syncQuality = 0
	}

	// Accept sync if quality is above threshold
	if d.syncQuality > 0.4 && bestPos >= 0 {
		d.syncFound++
		d.syncLocked = true
		// Realign buffer: remove samples before sync
		if bestPos > 0 {
			d.pixelBuf = d.pixelBuf[bestPos:]
			d.lineOffset = len(d.pixelBuf)
		}
	}
}

// finalizeLine converts the current pixel buffer to a byte line and resets.
func (d *APTDecoder) finalizeLine() (APTLine, bool) {
	if len(d.pixelBuf) < LinePixels {
		return APTLine{}, false
	}

	// Take exactly LinePixels samples
	line := make([]byte, LinePixels)

	// Normalize pixel values to 0-255
	// Find min/max for auto-scaling
	var minV, maxV float64
	minV = d.pixelBuf[0]
	maxV = d.pixelBuf[0]
	for i := 0; i < LinePixels; i++ {
		v := d.pixelBuf[i]
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}

	rangeV := maxV - minV
	if rangeV < 1e-6 {
		rangeV = 1
	}

	for i := 0; i < LinePixels; i++ {
		v := (d.pixelBuf[i] - minV) / rangeV
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

	// Store in image buffer
	if len(d.lines) < d.maxLines {
		d.lines = append(d.lines, line)
	} else {
		// Circular buffer: replace oldest
		idx := d.lineCount % d.maxLines
		d.lines[idx] = line
	}

	// Reset for next line
	d.pixelBuf = d.pixelBuf[LinePixels:]
	d.lineOffset = len(d.pixelBuf)

	// Periodically try to re-sync (every 100 lines)
	if d.lineCount%100 == 0 {
		d.syncLocked = false
	}

	return aptLine, true
}

// Stats returns decoder statistics.
func (d *APTDecoder) Stats() (linesDecoded, syncFound int) {
	return d.linesDecoded, d.syncFound
}

// GetImage returns the accumulated image as a 2D byte slice.
func (d *APTDecoder) GetImage() [][]byte {
	return d.lines
}

// Reset clears the decoder state and image buffer.
func (d *APTDecoder) Reset() {
	d.pixelBuf = d.pixelBuf[:0]
	d.lineCount = 0
	d.linesDecoded = 0
	d.syncFound = 0
	d.syncLocked = false
	d.lineOffset = 0
	d.lines = d.lines[:0]
	d.decimPhase = 0
	d.decimAccum = 0
	d.decimCount = 0
	// Reset filter states
	d.bpSubcarrier.z1 = 0
	d.bpSubcarrier.z2 = 0
	d.lpEnvelope.z1 = 0
	d.lpEnvelope.z2 = 0
}
