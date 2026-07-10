package adsb

import (
	"fmt"
	"math"
)

// decodeMessage parses a 14-byte (112-bit) ADS-B message and returns
// a Message struct with extracted fields.
func decodeMessage(msg []byte) *Message {
	if len(msg) != 14 {
		return nil
	}

	m := &Message{
		Raw:    make([]byte, 14),
		ME:     make([]byte, 7),
		Parity: make([]byte, 3),
	}
	copy(m.Raw, msg)
	copy(m.ME, msg[4:11])
	copy(m.Parity, msg[11:14])

	// Downlink format (first 5 bits)
	m.DF = int(msg[0] >> 3)
	m.CA = int(msg[0] & 0x07)

	// ICAO address (bytes 1-3)
	m.ICAO = decodeHexAddress(msg[1:4])

	// Type code (first 5 bits of ME)
	m.TypeCode = int(m.ME[0] >> 3)

	return m
}

// decodeCallsign extracts the 8-character callsign from an identification
// message (type codes 1-4).
func decodeCallsign(me []byte) string {
	if len(me) < 7 {
		return ""
	}
	const charset = "#ABCDEFGHIJKLMNOPQRSTUVWXYZ##### ###############0123456789######"
	var callsign [8]byte

	// The callsign is encoded in 48 bits (6 bytes) starting at bit 8 of ME
	// Each character is 6 bits
	bits := make([]int, 48)
	for i := range 6 {
		for j := range 8 {
			bits[i*8+j] = int((me[1+i] >> (7 - j)) & 1)
		}
	}
	// Reinterpret as 8 x 6-bit chars
	for i := range 8 {
		val := 0
		for j := range 6 {
			val = val<<1 | bits[i*6+j]
		}
		if val < len(charset) {
			callsign[i] = charset[val]
		}
	}

	// Trim trailing spaces and # characters
	result := string(callsign[:])
	for len(result) > 0 && (result[len(result)-1] == ' ' || result[len(result)-1] == '#') {
		result = result[:len(result)-1]
	}
	return result
}

// decodeAltitude extracts altitude in feet from an airborne position message.
func decodeAltitude(me []byte) int {
	if len(me) < 7 {
		return 0
	}
	// ME bit numbering (1-indexed, MSB of ME[0] = bit 1):
	//   ME[0]=bits 1-8, ME[1]=9-16, ME[2]=17-24, ME[3]=25-32,
	//   ME[4]=33-40, ME[5]=41-48, ME[6]=49-56.
	// Altitude occupies ME bits 9-20 (12 bits):
	//   ME[1] (bits 9-16) + the top 4 bits of ME[2] (bits 17-20).
	// The Q bit is ME bit 16, i.e. the LSB of ME[1] (me[1] & 0x01).
	qBit := me[1] & 0x01

	if qBit == 1 {
		// 25-foot encoding: N is an 11-bit value formed from
		//   ME bits 9-15  (ME[1] bits 7-1) and
		//   ME bits 17-20 (ME[2] bits 7-4).
		// (me[1]&0xFE)<<3 == (me[1]>>1)<<4 places the 7 high bits at
		// positions 4-10; me[2]>>4 supplies the low 4 bits at positions 0-3.
		n := int(me[1]&0xFE)<<3 | int(me[2]>>4)
		return n*25 - 1000
	}

	// Gillham coded altitude (100-foot steps) — rare, approximate
	return 0
}

// decodeAirbornePosition extracts the encoded position from an airborne
// position message. Returns (latEncoded, lonEncoded, isOdd).
func decodeAirbornePosition(me []byte) (lat, lon int, isOdd bool) {
	if len(me) < 7 {
		return 0, 0, false
	}

	// ME byte indices (ME bit 1 = MSB of ME[0]):
	//   ME[0]=bits 1-8, ME[1]=9-16, ME[2]=17-24, ME[3]=25-32,
	//   ME[4]=33-40, ME[5]=41-48, ME[6]=49-56.
	//
	// Bit 22 (CPR format F: 0=even, 1=odd) = ME[2] bit 2 -> (me[2]>>2)&1.

	// CPR format flag: 0=even, 1=odd
	isOdd = (me[2]>>2)&1 == 1

	// CPR latitude: ME bits 23-39 (17 bits)
	//   bits 23-24 = ME[2] bits 1-0 (2 bits, MSB=bit 23)
	//   bits 25-32 = ME[3] (8 bits)
	//   bits 33-39 = ME[4] bits 7-1 (7 bits, MSB=bit 33)
	lat = int(me[2]&0x03)<<15 | int(me[3])<<7 | int(me[4]>>1)

	// CPR longitude: ME bits 40-56 (17 bits)
	//   bit 40 = ME[4] bit 0 (1 bit, MSB)
	//   bits 41-48 = ME[5] (8 bits)
	//   bits 49-56 = ME[6] (8 bits)
	lon = int(me[4]&0x01)<<16 | int(me[5])<<8 | int(me[6])

	return lat, lon, isOdd
}

// decodeVelocity extracts speed, track/heading, vertical rate from
// an airborne velocity message (type code 19).
// Invalid/absent fields are returned as NaN so callers can distinguish
// a genuine zero (e.g. a due-north track) from "no information".
func decodeVelocity(me []byte) (speed, track, vRate float64) {
	speed = math.NaN()
	track = math.NaN()
	vRate = math.NaN()

	if len(me) < 7 {
		return
	}

	// Subtype (ST): bits 6-8 of ME → ME[0] & 0x07
	// TC is bits 1-5 (me[0] >> 3), ST is bits 6-8 (me[0] & 0x07)
	st := me[0] & 0x07

	switch st {
	case 1, 2:
		// Ground speed (subtypes 1, 2)
		// EW velocity: sign at bit 14, value at bits 15-24 (10 bits)
		ewSign := (me[1] >> 2) & 1
		ewRaw := int(me[1]&0x03)<<8 | int(me[2])
		// NS velocity: sign at bit 25, value at bits 26-35 (10 bits)
		nsSign := (me[3] >> 7) & 1
		nsRaw := int(me[3]&0x7F)<<3 | int(me[4]>>5)

		// A raw value of 0 means "no information".
		if ewRaw == 0 || nsRaw == 0 {
			break
		}

		ewVel := ewRaw - 1
		if ewSign == 1 {
			ewVel = -ewVel
		}
		nsVel := nsRaw - 1
		if nsSign == 1 {
			nsVel = -nsVel
		}

		scale := 1
		if st == 2 {
			scale = 4 // Supersonic
		}

		speed = math.Sqrt(float64(ewVel*ewVel+nsVel*nsVel)) * float64(scale)
		track = math.Mod(math.Atan2(float64(ewVel), float64(nsVel))*180/math.Pi+360, 360)

	case 3, 4:
		// Airspeed (subtypes 3, 4)
		// Heading: status at bit 14, value at bits 15-24 (10 bits)
		headStatus := (me[1] >> 2) & 1
		if headStatus == 1 {
			track = float64(int(me[1]&0x03)<<8|int(me[2])) * 360.0 / 1024.0
		}
		// Airspeed: value at bits 26-35 (10 bits); bit 25 selects IAS (0) / TAS (1)
		aspRaw := int(me[3]&0x7F)<<3 | int(me[4]>>5)
		if aspRaw > 0 {
			speed = float64(aspRaw - 1)
			if st == 4 {
				speed *= 4
			}
		}
	}

	// Vertical rate: source at bit 36, sign at bit 37, value at bits 38-46 (9 bits).
	// VR = (value - 1) * 64 ft/min; a raw value of 0 means "no information".
	vrSign := (me[4] >> 3) & 1
	vrRaw := int(me[4]&0x07)<<6 | int(me[5]>>2)
	if vrRaw > 0 {
		vrRaw--
		if vrSign == 1 {
			vRate = -float64(vrRaw) * 64
		} else {
			vRate = float64(vrRaw) * 64
		}
	}

	return speed, track, vRate
}

// String returns a human-readable representation of a message.
func (m *Message) String() string {
	return fmt.Sprintf("ICAO:%s TC:%d DF:%d", m.ICAO, m.TypeCode, m.DF)
}
