package adsb

import (
	"math"
	"testing"
)

// TestNLFunctionKnownLatitudes verifies the cprNLFunction lookup table
// against known correct values (1090-WP-9-14). The previous linear
// approximation returned NL=40 for Shanghai's latitude (~31.2N) instead
// of the correct NL=51, which corrupted longitude decoding for most of
// the globe.
func TestNLFunctionKnownLatitudes(t *testing.T) {
	cases := []struct {
		lat  float64
		want float64
	}{
		{0, 59},
		{31.2, 51},     // Shanghai
		{25.0, 54},     // Taiwan
		{52.25720, 36}, // Amsterdam CPR test vector latitude
		{89.9, 1},
	}
	for _, c := range cases {
		got := nl(c.lat)
		if got != c.want {
			t.Errorf("nl(%v) = %v, want %v", c.lat, got, c.want)
		}
	}
}

// TestDecodeCPRGlobalKnownVector verifies global CPR decoding by encoding a
// known position (Shanghai) into even/odd CPR frames and checking that
// decodeCPRGlobal recovers it. This exercises the exact same j/m formulas
// as dump1090's decodeCPR(), including the cprMod wraparound.
func TestDecodeCPRGlobalKnownVector(t *testing.T) {
	const cpr17 = 131072.0
	wantLat := 31.2304
	wantLon := 121.4737

	encode := func(isOdd bool) (int, int) {
		dLat := 360.0 / 60.0
		if isOdd {
			dLat = 360.0 / 59.0
		}
		latCPR := int(math.Mod(wantLat, dLat) / dLat * cpr17)
		dLon := 360.0 / nl(wantLat)
		if isOdd {
			dLon = 360.0 / math.Max(nl(wantLat)-1, 1)
		}
		lonCPR := int(math.Mod(wantLon, dLon) / dLon * cpr17)
		return latCPR, lonCPR
	}

	evenLat, evenLon := encode(false)
	oddLat, oddLon := encode(true)

	lat, lon, ok := decodeCPRGlobal(evenLat, evenLon, oddLat, oddLon, true)
	if !ok {
		t.Fatal("decodeCPRGlobal returned ok=false for a valid encoded pair")
	}
	if math.Abs(lat-wantLat) > 0.01 {
		t.Errorf("lat = %v, want ~%v", lat, wantLat)
	}
	if math.Abs(lon-wantLon) > 0.01 {
		t.Errorf("lon = %v, want ~%v", lon, wantLon)
	}
}

// TestDecodeCPRRelativeNearShanghai verifies that local (relative) CPR
// decoding produces a position close to a receiver located near Shanghai,
// using a synthetic CPR encoding of a point a few km away.
func TestDecodeCPRRelativeNearShanghai(t *testing.T) {
	const cpr17 = 131072.0
	refLat := 31.2304
	refLon := 121.4737

	// Encode a position ~10km north of the reference using the even-format
	// zone size, then verify relative decoding recovers it accurately.
	targetLat := refLat + 0.1
	targetLon := refLon + 0.1

	dLat := 360.0 / 60.0
	latCPR := int(math.Mod(targetLat, dLat) / dLat * cpr17)
	dLon := 360.0 / nl(targetLat)
	lonCPR := int(math.Mod(targetLon, dLon) / dLon * cpr17)

	lat, lon, ok := decodeCPRRelative(latCPR, lonCPR, false, refLat, refLon)
	if !ok {
		t.Fatal("decodeCPRRelative returned ok=false")
	}
	if math.Abs(lat-targetLat) > 0.01 {
		t.Errorf("lat = %v, want ~%v", lat, targetLat)
	}
	if math.Abs(lon-targetLon) > 0.01 {
		t.Errorf("lon = %v, want ~%v", lon, targetLon)
	}
}
