package lrpt

// packetParser reassembles CCSDS space packets from the MPDU data zones
// of consecutive VCDUs and decodes MSU-MR image packets (APID 64-69)
// into ImageSegments.
//
// Packet layout: 6-byte primary header (version/APID, sequence, length),
// 8-byte timestamp (day u16, ms u32, µs u16, big-endian), then the MCU
// segment: MCU index (1), scan header (2), segment header (3, byte 2 =
// JPEG quality factor), followed by the JPEG bitstream for 14 MCUs.
const (
	pktHdrLen      = 6
	pktTimeLen     = 8
	pktMCUHdrLen   = 6
	pktMaxLen      = pktHdrLen + pktTimeLen + 2048
	apidIdle       = 2047
	vcduCtrInvalid = -1
)

type packetParser struct {
	jpeg *jpegDecoder

	synced  bool
	vcduCtr int // last VCDU counter, -1 = unknown
	partial []byte

	// Strip allocation: MSU-MR scans one 8-line strip per ~1.232 s;
	// packets of the same strip share an onboard timestamp. Strips are
	// allocated globally (across APIDs) from timestamp deltas so that
	// channels stay row-aligned.
	hasT0    bool
	stripT   float64 // timestamp (ms) of the current strip
	stripIdx int

	packets int
	apids   map[int]bool
}

func newPacketParser() *packetParser {
	return &packetParser{
		jpeg:    newJPEGDecoder(),
		vcduCtr: vcduCtrInvalid,
		apids:   map[int]bool{},
	}
}

func (p *packetParser) reset() {
	p.synced = false
	p.vcduCtr = vcduCtrInvalid
	p.partial = p.partial[:0]
	p.hasT0 = false
	p.stripIdx = 0
	p.packets = 0
	p.apids = map[int]bool{}
}

// processVCDU consumes one 892-byte VCDU and returns decoded image
// segments.
func (p *packetParser) processVCDU(vcdu []byte) []ImageSegment {
	var segs []ImageSegment
	if len(vcdu) != VCDULen {
		return nil
	}

	ctr := int(vcdu[2])<<16 | int(vcdu[3])<<8 | int(vcdu[4])
	if p.vcduCtr != vcduCtrInvalid && ctr != (p.vcduCtr+1)&0xFFFFFF {
		// Lost VCDUs: any partial packet is unusable.
		p.synced = false
		p.partial = p.partial[:0]
	}
	p.vcduCtr = ctr

	hdrPtr := int(vcdu[MPDUDataOffset-2]&0x07)<<8 | int(vcdu[MPDUDataOffset-1])
	zone := vcdu[MPDUDataOffset : MPDUDataOffset+MPDUDataLen]

	off := 0
	if !p.synced {
		if hdrPtr == 0x7FF {
			return nil // no packet starts here
		}
		if hdrPtr >= MPDUDataLen {
			return nil
		}
		off = hdrPtr
		p.synced = true
		p.partial = p.partial[:0]
	}

	for off < len(zone) {
		if len(p.partial) < pktHdrLen {
			// still collecting the header
			need := pktHdrLen - len(p.partial)
			take := min(need, len(zone)-off)
			p.partial = append(p.partial, zone[off:off+take]...)
			off += take
			if len(p.partial) < pktHdrLen {
				return segs
			}
		}
		total := pktHdrLen + int(p.partial[4])<<8 + int(p.partial[5]) + 1
		if total > pktMaxLen {
			// Corrupt length: drop sync, re-acquire via next header ptr.
			p.synced = false
			p.partial = p.partial[:0]
			return segs
		}
		take := min(total-len(p.partial), len(zone)-off)
		p.partial = append(p.partial, zone[off:off+take]...)
		off += take
		if len(p.partial) < total {
			return segs
		}
		if seg := p.handlePacket(p.partial); seg != nil {
			segs = append(segs, *seg)
		}
		p.partial = p.partial[:0]
	}
	return segs
}

// handlePacket decodes one complete CCSDS packet; returns an image
// segment for MSU-MR image APIDs, nil otherwise.
func (p *packetParser) handlePacket(pkt []byte) *ImageSegment {
	apid := int(pkt[0]&0x07)<<8 | int(pkt[1])
	if apid == apidIdle || apid < APIDImageMin || apid > APIDImageMax {
		return nil
	}
	if len(pkt) < pktHdrLen+pktTimeLen+pktMCUHdrLen+1 {
		return nil
	}

	// Timestamp: day (u16), ms of day (u32), µs (u16)
	day := int64(pkt[6])<<8 | int64(pkt[7])
	ms := int64(pkt[8])<<24 | int64(pkt[9])<<16 | int64(pkt[10])<<8 | int64(pkt[11])
	t := float64(day)*86400000 + float64(ms)

	mcuHdr := pkt[pktHdrLen+pktTimeLen:]
	mcuIdx := int(mcuHdr[0])
	q := int(mcuHdr[5])
	if mcuIdx+MCUPerPacket > MCUPerLine {
		return nil
	}

	strip := p.allocStrip(t)

	pixels := make([]byte, StripHeight*MCUPerPacket*8)
	jpegData := pkt[pktHdrLen+pktTimeLen+pktMCUHdrLen:]
	if !p.jpeg.decodeMCUs(jpegData, q, MCUPerPacket, pixels) {
		return nil
	}

	p.packets++
	p.apids[apid] = true
	return &ImageSegment{
		APID:     apid,
		Strip:    strip,
		MCUIndex: mcuIdx,
		Pixels:   pixels,
	}
}

// allocStrip maps an onboard timestamp to a strip index.
func (p *packetParser) allocStrip(t float64) int {
	if !p.hasT0 {
		p.hasT0 = true
		p.stripT = t
		p.stripIdx = 0
		return 0
	}
	dt := t - p.stripT
	if dt < StripPeriodMs*0.55 {
		// Same strip (also covers small negative jitter and packets of
		// other APIDs from the same scan).
		return p.stripIdx
	}
	adv := int(dt/StripPeriodMs + 0.5)
	if adv < 1 {
		adv = 1
	}
	if adv > 200 {
		// Implausible jump (corrupt timestamp survived RS, or a very
		// long gap): restart close to the current position.
		adv = 1
	}
	p.stripIdx += adv
	p.stripT = t
	return p.stripIdx
}
