package adsb

import (
	"encoding/hex"
	"testing"
)

// encodeAirbornePositionME packs (tc, latEnc, lonEnc, isOdd) into a 7-byte ME
// using the standard ADS-B airborne-position bit layout (the inverse of
// decodeAirbornePosition). Used to round-trip test the decoder.
func encodeAirbornePositionME(tc int, latEnc, lonEnc int, isOdd bool) []byte {
	me := make([]byte, 7)
	me[0] = byte(tc << 3) // TC in ME bits 1-5
	if isOdd {
		me[2] |= 1 << 2 // F flag at ME bit 22 = me[2] bit 2
	}
	me[2] |= byte((latEnc >> 15) & 0x03) // lat bits 23-24 -> me[2] bits 1-0
	me[3] = byte((latEnc >> 7) & 0xFF)    // lat bits 25-32 -> me[3]
	me[4] |= byte((latEnc & 0x7F) << 1)   // lat bits 33-39 -> me[4] bits 7-1
	me[4] |= byte((lonEnc >> 16) & 0x01)  // lon bit 40 -> me[4] bit 0
	me[5] = byte((lonEnc >> 8) & 0xFF)    // lon bits 41-48 -> me[5]
	me[6] = byte(lonEnc & 0xFF)           // lon bits 49-56 -> me[6]
	return me
}

// Real-world airborne-position message 8D40621D58C382D690C5AC (ICAO 40621D,
// TC=11 baro, even format). Its altitude is 38000 ft.
var realAirborneMsg, _ = hex.DecodeString("8D40621D58C382D690C5AC")

func TestDecodeAltitudeRealMessage(t *testing.T) {
	msg := realAirborneMsg
	if len(msg) < 11 {
		t.Fatalf("bad test vector length %d", len(msg))
	}
	me := msg[4:11]
	alt := decodeAltitude(me)
	if alt != 38000 {
		t.Fatalf("altitude: got %d, want 38000 (ME=% X)", alt, me)
	}
}

func TestDecodeAirbornePositionRoundTrip(t *testing.T) {
	cases := []struct {
		tc     int
		latEnc int
		lonEnc int
		isOdd  bool
	}{
		{11, 0x12345, 0x0ABCD, true},
		{11, 0x00001, 0x1FFFE, false},
		{11, 0x1FFFF, 0x1FFFF, true},
		{11, 0, 0, false},
		{15, 74565, 43981, true},
	}
	for i, c := range cases {
		me := encodeAirbornePositionME(c.tc, c.latEnc, c.lonEnc, c.isOdd)
		lat, lon, isOdd := decodeAirbornePosition(me)
		if lat != c.latEnc || lon != c.lonEnc || isOdd != c.isOdd {
			t.Errorf("case %d: got (lat=%d lon=%d isOdd=%v), want (lat=%d lon=%d isOdd=%v) [ME=% X]",
				i, lat, lon, isOdd, c.latEnc, c.lonEnc, c.isOdd, me)
		}
	}
}

func TestDecodeAirbornePositionRealEvenFormat(t *testing.T) {
	// The real message above is an EVEN-format airborne position (F=0).
	me := realAirborneMsg[4:11]
	_, _, isOdd := decodeAirbornePosition(me)
	if isOdd {
		t.Fatal("real even-format message decoded as odd")
	}
}
