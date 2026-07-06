package adsb

// CRC polynomial for Mode S / ADS-B: 0x1FFF409 (24-bit)
// Generator polynomial: G(x) = x^24 + x^23 + x^20 + x^17 + x^13 + x^12 + x^11 +
//                            x^10 + x^7 + x^5 + x^4 + x^2 + x + 1
// The 24 most-significant bits of the CRC register form the checksum.

// crcTable is precomputed for the Mode S CRC.
var crcTable [256]uint32

// crcGenerator is the Mode S CRC generator polynomial (without the leading 1).
const crcGenerator = 0xFF_FFF4 // 24-bit polynomial

func init() {
	for i := range 256 {
		crc := uint32(i) << 16
		for j := 0; j < 8; j++ {
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

// fixSingleBitError attempts to fix a single-bit error in the message.
// Returns the corrected message and true if a fix was applied.
func fixSingleBitError(msg []byte) ([]byte, bool) {
	if len(msg) != 14 {
		return msg, false
	}
	crc := computeCRC(msg[:11])
	parity := uint32(msg[11])<<16 | uint32(msg[12])<<8 | uint32(msg[13])
	_ = crc ^ parity // syndrome

	// Try flipping each bit and check CRC
	for byteIdx := range 11 {
		for bitIdx := 0; bitIdx < 8; bitIdx++ {
			fixed := make([]byte, len(msg))
			copy(fixed, msg)
			fixed[byteIdx] ^= 1 << (7 - bitIdx)
			if checkCRC(fixed) {
				return fixed, true
			}
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
