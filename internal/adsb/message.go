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
	const charset = "#ABCDEFGHIJKLMNOPQRSTUVWXYZ#####_###############0123456789######"
	var callsign [8]byte

	// The callsign is encoded in 48 bits (6 bytes) starting at bit 8 of ME
	// Each character is 6 bits
	bits := make([]int, 48)
	for i := 0; i < 6; i++ {
		for j := 0; j < 8; j++ {
			bits[i*8+j] = int((me[1+i] >> (7 - j)) & 1)
		}
	}
	// Reinterpret as 8 x 6-bit chars
	for i := 0; i < 8; i++ {
		val := 0
		for j := 0; j < 6; j++ {
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
	// Altitude is encoded in bits 33-45 of ME (13 bits)
	// Bit 40 (Q bit) determines encoding: Q=1 → 25ft encoding, Q=0 → 100ft (Gillham)
	// We read bits from the 2nd byte of ME
	// ME[5] bits 0-7, ME[6] bits 0-4 → bits 33-45 of ME

	// Extract the 12-bit AC field (bits 40-51 of the message = bits 0-11 of ME[5..6])
	// Actually, altitude is in bits 33-45 of the 56-bit ME field
	// ME byte indices: ME[0]=bits 0-7, ME[1]=8-15, ME[2]=16-23, ME[3]=24-31,
	//                  ME[4]=32-39, ME[5]=40-47, ME[6]=48-55

	// The altitude field starts at ME bit 33 (5th bit of ME[4]) and is 12 bits long
	// But the standard encoding uses:
	// - ME[5] bits 6-0 (7 bits) + ME[6] bits 7-6 (2 bits) = 9-bit mantissa when Q=1
	// Actually, let me use the standard bit extraction:

	// Bits 40-52 of the ADS-B message (ME bits 40-52) encode altitude
	// Q bit is at bit 48 (ME[5] bit 0)
	qBit := (me[5] >> 0) & 0x01

	if qBit == 1 {
		// 25-foot encoding
		// N = (ME[5] & 0xFE) << 3 | (ME[6] >> 5)
		// But we need to extract bits properly
		// Bits 40-46 (7 bits, excluding Q bit at 48) and bits 49-52 (4 bits)
		n := int(me[5]&0xFE)<<3 | int(me[6]>>5)
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

	// ME is 56 bits (7 bytes). Bit 1 = MSB of ME[0].
	//   Bits 1-5:   Type Code (TC)
	//   Bits 6-7:   Surveillance status (SS)
	//   Bit 8:      NIC supplement
	//   Bits 9-20:  Altitude (12 bits)
	//   Bit 21:     Time
	//   Bit 22:     CPR format (F flag: 0=even, 1=odd)
	//   Bits 23-39: CPR latitude (17 bits)
	//   Bits 40-56: CPR longitude (17 bits)
	//
	// ME[0]=bits 1-8, ME[1]=bits 9-16, ME[2]=bits 17-24, etc.
	// Bit 22 = ME[2] bit 6 from MSB → (me[2] >> 1) & 1

	// CPR format flag: 0=even, 1=odd
	isOdd = (me[2]>>1)&1 == 1

	// CPR latitude: bits 23-39 = 17 bits
	// ME[2] bits 7-8 (2 bits) + ME[3] (8 bits) + ME[4] bits 1-7 (7 bits)
	lat = int(me[2]&0x01)<<15 | int(me[3])<<7 | int(me[4]>>1)

	// CPR longitude: bits 40-56 = 17 bits
	// ME[4] bit 8 (1 bit) + ME[5] (8 bits) + ME[6] bits 1-8 (8 bits)
	lon = int(me[4]&0x01)<<16 | int(me[5])<<8 | int(me[6])

	return lat, lon, isOdd
}

// decodeVelocity extracts speed, track/heading, vertical rate from
// an airborne velocity message (type code 19).
func decodeVelocity(me []byte) (speed, track, vRate float64) {
	if len(me) < 7 {
		return 0, 0, 0
	}

	// Subtype (ST): bits 6-8 of ME → ME[0] & 0x07
	// TC is bits 1-5 (me[0] >> 3), ST is bits 6-8 (me[0] & 0x07)
	st := me[0] & 0x07

	switch st {
	case 1, 2:
		// Ground speed (subtypes 1, 2)
		// EW velocity: bits 14-23 (10 bits, signed), sign at bit 14
		// NS velocity: bits 25-34 (10 bits, signed), sign at bit 25
		ewSign := (me[1] >> 2) & 1
		ewVel := int(me[1]&0x03)<<8 | int(me[2])
		ewVel--
		if ewSign == 1 {
			ewVel = -ewVel
		}

		nsSign := (me[3] >> 2) & 1
		nsVel := int(me[3]&0x03)<<8 | int(me[4])
		nsVel--
		if nsSign == 1 {
			nsVel = -nsVel
		}

		speed = math.Sqrt(float64(ewVel*ewVel + nsVel*nsVel))
		track = math.Mod(math.Atan2(float64(ewVel), float64(nsVel))*180/math.Pi+360, 360)

		if st == 2 {
			// Supersonic, scale by 4
			speed *= 4
		}
	case 3, 4:
		// Airspeed (subtypes 3, 4)
		// Heading: bits 15-24 (10 bits), bit 14 is status (HST)
		headStatus := (me[1] >> 2) & 1
		if headStatus == 1 {
			track = float64(int(me[1]&0x03)<<8|int(me[2])) * 360.0 / 1024.0
		}
		// Airspeed: bits 26-35 (10 bits)
		// Bit 26 indicates IAS (0) or TAS (1)
		speed = float64(int(me[3]&0x03)<<8 | int(me[4]))
		if st == 4 {
			speed *= 4
		}
	}

	// Vertical rate: bits 37-46 (10 bits), sign at bit 37, VR = (value - 1) * 64 ft/min
	vrSign := (me[4] >> 2) & 1
	vrRaw := int(me[4]&0x03)<<7 | int(me[5]>>1)
	vrRaw--
	if vrSign == 1 {
		vRate = -float64(vrRaw) * 64
	} else {
		vRate = float64(vrRaw) * 64
	}

	return speed, track, vRate
}

// String returns a human-readable representation of a message.
func (m *Message) String() string {
	return fmt.Sprintf("ICAO:%s TC:%d DF:%d", m.ICAO, m.TypeCode, m.DF)
}
