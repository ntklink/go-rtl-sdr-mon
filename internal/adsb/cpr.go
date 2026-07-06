package adsb

import "math"

// CPR (Compact Position Reporting) decoding for ADS-B position messages.
//
// References:
//   ICAO Annex 10 Vol III, Chapter 9, §9.4.3.4
//   RTCA DO-260B §2.4.3.8.5

const (
	cprNZ = 15 // Number of latitude zones (usually 15)
)

func nl(lat float64) float64 {
	// Number of longitude zones at a given latitude
	if lat < 0 {
		lat = -lat
	}
	if lat >= 87.0 {
		return 1
	}
	if lat >= 86.5 {
		return 2
	}
	if lat >= 85.5 {
		return 3
	}
	if lat >= 84.5 {
		return 4
	}
	if lat >= 83.5 {
		return 5
	}
	if lat >= 82.5 {
		return 6
	}
	if lat >= 81.5 {
		return 7
	}
	if lat >= 80.5 {
		return 8
	}
	if lat >= 79.5 {
		return 9
	}
	if lat >= 78.5 {
		return 10
	}
	if lat >= 77.5 {
		return 11
	}
	if lat >= 76.5 {
		return 12
	}
	if lat >= 75.5 {
		return 13
	}
	if lat >= 74.5 {
		return 14
	}
	if lat >= 73.5 {
		return 15
	}
	if lat >= 72.5 {
		return 16
	}
	if lat >= 71.5 {
		return 17
	}
	if lat >= 70.5 {
		return 18
	}
	if lat >= 69.5 {
		return 19
	}
	if lat >= 68.5 {
		return 20
	}
	if lat >= 67.5 {
		return 21
	}
	if lat >= 66.5 {
		return 22
	}
	if lat >= 64.5 {
		return 23
	}
	if lat >= 62.5 {
		return 24
	}
	if lat >= 60.5 {
		return 25
	}
	if lat >= 58.5 {
		return 26
	}
	if lat >= 56.5 {
		return 27
	}
	if lat >= 54.5 {
		return 28
	}
	if lat >= 52.5 {
		return 29
	}
	if lat >= 50.5 {
		return 30
	}
	if lat >= 48.5 {
		return 31
	}
	if lat >= 46.5 {
		return 32
	}
	if lat >= 44.5 {
		return 33
	}
	if lat >= 42.5 {
		return 34
	}
	if lat >= 40.5 {
		return 35
	}
	if lat >= 38.5 {
		return 36
	}
	if lat >= 36.5 {
		return 37
	}
	if lat >= 34.5 {
		return 38
	}
	if lat >= 32.5 {
		return 39
	}
	if lat >= 30.5 {
		return 40
	}
	if lat >= 28.5 {
		return 41
	}
	if lat >= 26.5 {
		return 42
	}
	if lat >= 24.5 {
		return 43
	}
	if lat >= 22.5 {
		return 44
	}
	if lat >= 20.5 {
		return 45
	}
	if lat >= 18.5 {
		return 46
	}
	if lat >= 16.5 {
		return 47
	}
	if lat >= 14.5 {
		return 48
	}
	if lat >= 12.5 {
		return 49
	}
	if lat >= 10.5 {
		return 50
	}
	if lat >= 8.5 {
		return 51
	}
	if lat >= 6.5 {
		return 52
	}
	if lat >= 4.5 {
		return 53
	}
	if lat >= 2.5 {
		return 54
	}
	return 59
}

// decodeCPRGlobal decodes a position from a pair of CPR messages (even + odd).
// Returns (latitude, longitude, ok).
func decodeCPRGlobal(latEven, lonEven, latOdd, lonOdd int) (lat, lon float64, ok bool) {
	const cprLat = 131072.0 // 2^17
	const cprLon = 131072.0

	// Compute latitude
	j := math.Floor(float64(59*latEven-cprNZ*latOdd)/cprLat + 0.5)

	// Even latitude
	dLatEven := 360.0 / (4 * cprNZ)
	latEvenVal := dLatEven * (float64(j) + float64(latEven)/cprLat)

	// Odd latitude
	dLatOdd := 360.0 / (4*cprNZ - 1)
	latOddVal := dLatOdd * (float64(j) + float64(latOdd)/cprLat)

	// Choose the correct latitude (use the latest message's parity)
	// For global decoding, compute both and select based on NL
	lat = latEvenVal
	if lat >= 270 {
		lat -= 360
	}

	// Check NL zone consistency
	nlLat := nl(lat)
	if nlLat < 1 {
		nlLat = 1
	}

	// Compute longitude
	ni := nlLat
	if ni < 1 {
		ni = 1
	}
	m := math.Floor(float64(lonEven*(int(nlLat)-1)-lonOdd*int(nlLat))/cprLon + 0.5)

	dLonEven := 360.0 / ni
	lonEvenVal := dLonEven * (float64(m) + float64(lonEven)/cprLon)

	// Odd longitude (not directly used, but computed for validation)
	niOdd := nlLat - 1
	if niOdd < 1 {
		niOdd = 1
	}
	dLonOdd := 360.0 / niOdd
	_ = dLonOdd * (float64(m) + float64(lonOdd)/cprLon) // lonOddVal

	lon = lonEvenVal
	if lon >= 180 {
		lon -= 360
	}

	// Validate
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		// Try odd latitude instead
		lat = latOddVal
		if lat >= 270 {
			lat -= 360
		}
		nlLat = nl(lat)
		ni = nlLat
		if ni < 1 {
			ni = 1
		}
		m = math.Floor(float64(lonEven*(int(nlLat)-1)-lonOdd*int(nlLat))/cprLon + 0.5)
		dLonEven = 360.0 / ni
		lon = dLonEven * (float64(m) + float64(lonEven)/cprLon)
		if lon >= 180 {
			lon -= 360
		}
		if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			return 0, 0, false
		}
	}

	return lat, lon, true
}

// decodeCPRRelative decodes a position from a single CPR message using a
// known receiver position. Returns (latitude, longitude, ok).
func decodeCPRRelative(latEnc, lonEnc int, isOdd bool, refLat, refLon float64) (lat, lon float64, ok bool) {
	const cpr = 131072.0

	dLat := 360.0 / (4*cprNZ - 1)
	if !isOdd {
		dLat = 360.0 / (4 * cprNZ)
	}

	j := math.Floor(refLat/dLat + float64(latEnc)/cpr - 0.5)
	lat = dLat * (float64(j) + float64(latEnc)/cpr)
	if lat >= 270 {
		lat -= 360
	}

	nlLat := nl(lat)
	if nlLat < 1 {
		nlLat = 1
	}

	ni := int(nlLat)
	if isOdd {
		ni = int(nlLat) - 1
	}
	if ni < 1 {
		ni = 1
	}

	dLon := 360.0 / float64(ni)
	m := math.Floor(refLon/dLon + float64(lonEnc)/cpr - 0.5)
	lon = dLon * (float64(m) + float64(lonEnc)/cpr)
	if lon >= 180 {
		lon -= 360
	}

	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0, false
	}

	return lat, lon, true
}
