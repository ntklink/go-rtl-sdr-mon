package adsb

import (
	"math/rand"
	"testing"
)

// bitByBitCRC is the canonical Mode S CRC (dump1090 modesChecksum) computed
// bit-by-bit over `nbits` of `data` (MSB first), using the full 25-bit
// generator 0x1FFF409. A valid 112-bit message yields 0.
func bitByBitCRC(data []byte, nbits int) uint32 {
	const poly = 0x1FFF409
	var crc uint32
	for i := 0; i < nbits; i++ {
		b := uint32((data[i/8] >> (7 - i%8)) & 1)
		crc = (crc << 1) | b
		if crc&0x1000000 != 0 {
			crc ^= poly
		}
	}
	return crc & 0xFFFFFF
}

func TestCRCTableMatchesBitByBit(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 1000; trial++ {
		var msg [14]byte
		rng.Read(msg[:11]) // random payload (first 88 bits)

		// Compute parity with the table implementation.
		crc := computeCRC(msg[:11])
		msg[11] = byte(crc >> 16)
		msg[12] = byte(crc >> 8)
		msg[13] = byte(crc)

		// Full-message bit-by-bit CRC must be 0 for a valid message.
		if got := bitByBitCRC(msg[:], 112); got != 0 {
			t.Fatalf("trial %d: table CRC %06X did not validate (bit-by-bit=%06X)", trial, crc, got)
		}

		// checkCRC must agree.
		if !checkCRC(msg[:]) {
			t.Fatalf("trial %d: checkCRC rejected a valid message", trial)
		}
	}
}

func TestCRCRejectsCorrupted(t *testing.T) {
	var msg [14]byte
	rng := rand.New(rand.NewSource(2))
	rng.Read(msg[:11])
	crc := computeCRC(msg[:11])
	msg[11] = byte(crc >> 16)
	msg[12] = byte(crc >> 8)
	msg[13] = byte(crc)

	if !checkCRC(msg[:]) {
		t.Fatal("baseline should be valid")
	}

	// Flip one payload bit -> must fail (unless correctable to the same bit).
	flipped := make([]byte, 14)
	copy(flipped, msg[:])
	flipped[3] ^= 0x40
	if checkCRC(flipped) {
		t.Fatal("corrupted message unexpectedly passed CRC")
	}
}

func TestFixSingleBitError(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for trial := 0; trial < 200; trial++ {
		var msg [14]byte
		rng.Read(msg[:11])
		crc := computeCRC(msg[:11])
		msg[11] = byte(crc >> 16)
		msg[12] = byte(crc >> 8)
		msg[13] = byte(crc)

		// Corrupt a single random payload bit (first 88 bits only).
		bit := rng.Intn(88)
		msg[bit/8] ^= 1 << (7 - bit%8)

		fixed, ok := fixSingleBitError(msg[:])
		if !ok {
			t.Fatalf("trial %d: single-bit error not corrected", trial)
		}
		if !checkCRC(fixed) {
			t.Fatalf("trial %d: corrected message failed CRC", trial)
		}
	}
}
