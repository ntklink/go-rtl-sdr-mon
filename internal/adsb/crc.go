package adsb

// CRC polynomial for Mode S / ADS-B (24-bit).
// Generator: G(x) = x^24 + (0xFFF409), the canonical Mode S polynomial
// (dump1090 MODES_GENERATOR_POLY 0x1FFF409). The 24 most-significant bits
// of the CRC register form the checksum.

// crcTable is precomputed for the Mode S CRC.
var crcTable [256]uint32

// crcGenerator is the Mode S CRC generator polynomial: the lower 24 bits of
// the canonical Mode S / ADS-B generator (dump1090's MODES_GENERATOR_POLY
// 0x1FFF409, i.e. x^24 plus the 24-bit value 0xFFF409). The implicit x^24
// term is cancelled by the masked left shift in the table below.
const crcGenerator = 0xFFF409 // 24-bit polynomial

func init() {
	for i := range 256 {
		crc := uint32(i) << 16
		for range 8 {
			if crc&0x800000 != 0 {
				crc = (crc << 1) ^ crcGenerator
			} else {
				crc <<= 1
			}
		}
		crcTable[i] = crc & 0xFFFFFF
	}
}

// computeCRC calculates the 24-bit Mode S CRC for a message.
// The message is the full 112-bit message (14 bytes) without the parity.
// The last 3 bytes are the parity to check against.
func computeCRC(data []byte) uint32 {
	crc := uint32(0)
	for _, b := range data {
		crc = (crc << 8) ^ crcTable[byte(crc>>16)^b]
	}
	return crc & 0xFFFFFF
}

// checkCRC verifies the CRC of a 14-byte message.
// Returns true if the CRC is valid.
func checkCRC(msg []byte) bool {
	if len(msg) != 14 {
		return false
	}
	crc := computeCRC(msg[:11])
	parity := uint32(msg[11])<<16 | uint32(msg[12])<<8 | uint32(msg[13])
	return crc == parity
}

// fixSingleBitError attempts to correct a single-bit error in the message
// by brute-forcing each payload bit and re-checking the CRC.
// Returns the corrected message and true if a fix was applied.
func fixSingleBitError(msg []byte) ([]byte, bool) {
	if len(msg) != 14 {
		return msg, false
	}

	// Work on a single scratch copy, flipping/un-flipping each candidate
	// bit in place, instead of allocating a fresh 14-byte slice per one of
	// up to 112 attempts (this runs on every CRC failure, which noise
	// makes common). msg[:11] is the CRC input for every attempt except
	// when byteIdx is in the parity bytes (11-13); computeCRC only reads
	// [:11], so parity-bit flips don't change that computation, but we
	// still need the parity bytes correct in the returned message.
	fixed := make([]byte, len(msg))
	copy(fixed, msg)

	// Try flipping each bit (all 112 bits = 14 bytes, including parity) and
	// check CRC. A single-bit error in the parity leaves the payload intact,
	// so those messages must be correctable too.
	for byteIdx := range 14 {
		for bitIdx := range 8 {
			bit := byte(1 << (7 - bitIdx))
			fixed[byteIdx] ^= bit
			if checkCRC(fixed) {
				return fixed, true
			}
			fixed[byteIdx] ^= bit // undo before trying the next bit
		}
	}
	return msg, false
}

// decodeHexAddress converts 3 bytes to a 6-digit hex ICAO address string.
func decodeHexAddress(addr []byte) string {
	if len(addr) < 3 {
		return ""
	}
	const hexDigits = "0123456789ABCDEF"
	b := []byte{
		hexDigits[addr[0]>>4], hexDigits[addr[0]&0xF],
		hexDigits[addr[1]>>4], hexDigits[addr[1]&0xF],
		hexDigits[addr[2]>>4], hexDigits[addr[2]&0xF],
	}
	return string(b)
}
