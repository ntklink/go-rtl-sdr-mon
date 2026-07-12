package lrpt

import "sort"

// Decoder is the top-level LRPT decoder: complex baseband IQ in,
// decoded MSU-MR image segments out.
type Decoder struct {
	demod   *Demodulator
	framer  *deframer
	packets *packetParser
}

// NewDecoder creates an LRPT decoder for the given IQ sample rate.
func NewDecoder(sampleRate float64) *Decoder {
	return &Decoder{
		demod:   NewDemodulator(sampleRate),
		framer:  newDeframer(),
		packets: newPacketParser(),
	}
}

// Process consumes baseband IQ samples (centered on the LRPT carrier)
// and returns any image segments completed during this block.
func (d *Decoder) Process(iq []complex128) []ImageSegment {
	soft := d.demod.Process(iq)
	if len(soft) == 0 {
		return nil
	}
	var segs []ImageSegment
	for _, vcdu := range d.framer.process(soft) {
		segs = append(segs, d.packets.processVCDU(vcdu)...)
	}
	return segs
}

// Stats returns current decoder statistics.
func (d *Decoder) Stats() Stats {
	apids := make([]int, 0, len(d.packets.apids))
	for a := range d.packets.apids {
		apids = append(apids, a)
	}
	sort.Ints(apids)
	return Stats{
		Locked:     d.demod.Locked(),
		SignalQ:    d.demod.Quality(),
		FreqOffset: d.demod.FreqOffset(),
		FramesOK:   d.framer.framesOK,
		FramesBad:  d.framer.framesBad,
		RSCorrect:  d.framer.rsCorr,
		Packets:    d.packets.packets,
		APIDs:      apids,
	}
}

// Constellation returns a snapshot of recent soft symbols as interleaved
// int8 I,Q pairs (for the UI constellation diagram).
func (d *Decoder) Constellation() []int8 {
	return d.demod.Constellation()
}

// Reset clears all decoder state (demodulator loops keep their tuning).
func (d *Decoder) Reset() {
	d.demod.Reset()
	d.framer.reset()
	d.packets.reset()
}
