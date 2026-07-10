package adsb

import "math"

// CPR (Compact Position Reporting) decoding for ADS-B position messages.
//
// References:
//   ICAO Annex 10 Vol III, Chapter 9, §9.4.3.4
//   RTCA DO-260B §2.4.3.8.5
//   1090-WP-9-14 (cprNLFunction lookup table)

// nl implements the ICAO cprNLFunction: the number of longitude zones (NL)
// at a given latitude. This must use the exact 1090-WP-9-14 table (59
// breakpoints); a coarse linear approximation produces wrong NL values
// for most latitudes (e.g. Shanghai ~31.2N: correct NL=51, an approximated
// table can give NL=40), which corrupts CPR longitude decoding.
func nl(lat float64) float64 {
	if lat < 0 {
		lat = -lat // table is symmetric about the equator
	}
	switch {
	case lat < 10.47047130:
		return 59
	case lat < 14.82817437:
		return 58
	case lat < 18.18626357:
		return 57
	case lat < 21.02939493:
		return 56
	case lat < 23.54504487:
		return 55
	case lat < 25.82924707:
		return 54
	case lat < 27.93898710:
		return 53
	case lat < 29.91135686:
		return 52
	case lat < 31.77209708:
		return 51
	case lat < 33.53993436:
		return 50
	case lat < 35.22899598:
		return 49
	case lat < 36.85025108:
		return 48
	case lat < 38.41241892:
		return 47
	case lat < 39.92256684:
		return 46
	case lat < 41.38651832:
		return 45
	case lat < 42.80914012:
		return 44
	case lat < 44.19454951:
		return 43
	case lat < 45.54626723:
		return 42
	case lat < 46.86733252:
		return 41
	case lat < 48.16039128:
		return 40
	case lat < 49.42776439:
		return 39
	case lat < 50.67150166:
		return 38
	case lat < 51.89342469:
		return 37
	case lat < 53.09516153:
		return 36
	case lat < 54.27817472:
		return 35
	case lat < 55.44378444:
		return 34
	case lat < 56.59318756:
		return 33
	case lat < 57.72747354:
		return 32
	case lat < 58.84763776:
		return 31
	case lat < 59.95459277:
		return 30
	case lat < 61.04917774:
		return 29
	case lat < 62.13216659:
		return 28
	case lat < 63.20427479:
		return 27
	case lat < 64.26616523:
		return 26
	case lat < 65.31845310:
		return 25
	case lat < 66.36171008:
		return 24
	case lat < 67.39646774:
		return 23
	case lat < 68.42322022:
		return 22
	case lat < 69.44242631:
		return 21
	case lat < 70.45451075:
		return 20
	case lat < 71.45986473:
		return 19
	case lat < 72.45884545:
		return 18
	case lat < 73.45177442:
		return 17
	case lat < 74.43893416:
		return 16
	case lat < 75.42056257:
		return 15
	case lat < 76.39684391:
		return 14
	case lat < 77.36789461:
		return 13
	case lat < 78.33374083:
		return 12
	case lat < 79.29428225:
		return 11
	case lat < 80.24923213:
		return 10
	case lat < 81.19801349:
		return 9
	case lat < 82.13956981:
		return 8
	case lat < 83.07199445:
		return 7
	case lat < 83.99173563:
		return 6
	case lat < 84.89166191:
		return 5
	case lat < 85.75541621:
		return 4
	case lat < 86.53536998:
		return 3
	case lat < 87.00000000:
		return 2
	default:
		return 1
	}
}

// cprMod is a always-positive modulo, used throughout CPR decoding.
func cprMod(a, b int) int {
	res := a % b
	if res < 0 {
		res += b
	}
	return res
}

// decodeCPRGlobal decodes a position from a pair of CPR messages (even + odd).
// useEven indicates whether the even-parity message is the more recently
// received one (dump1090 uses the newer message's zone to resolve longitude).
// Returns (latitude, longitude, ok).
func decodeCPRGlobal(latEven, lonEven, latOdd, lonOdd int, useEven bool) (lat, lon float64, ok bool) {
	const cpr17 = 131072.0 // 2^17

	const airDlat0 = 360.0 / 60.0 // even-parity latitude zone size
	const airDlat1 = 360.0 / 59.0 // odd-parity latitude zone size

	lat0 := float64(latEven)
	lat1 := float64(latOdd)
	lon0 := float64(lonEven)
	lon1 := float64(lonOdd)

	// Latitude index, common to both even and odd zones.
	j := math.Floor((59*lat0-60*lat1)/cpr17 + 0.5)

	rlat0 := airDlat0 * (float64(cprMod(int(j), 60)) + lat0/cpr17)
	rlat1 := airDlat1 * (float64(cprMod(int(j), 59)) + lat1/cpr17)

	if rlat0 >= 270 {
		rlat0 -= 360
	}
	if rlat1 >= 270 {
		rlat1 -= 360
	}

	if rlat0 < -90 || rlat0 > 90 || rlat1 < -90 || rlat1 > 90 {
		return 0, 0, false
	}

	// Both messages must fall in the same NL zone, otherwise the aircraft
	// moved between zones during the even/odd interval and the pair is
	// unusable.
	nlRlat0 := nl(rlat0)
	nlRlat1 := nl(rlat1)
	if nlRlat0 != nlRlat1 {
		return 0, 0, false
	}

	var ni, m float64
	if useEven {
		ni = math.Max(nlRlat0, 1)
		m = math.Floor((lon0*(nlRlat0-1)-lon1*nlRlat0)/cpr17 + 0.5)
		dLon := 360.0 / ni
		lon = dLon * (float64(cprMod(int(m), int(ni))) + lon0/cpr17)
		lat = rlat0
	} else {
		ni = math.Max(nlRlat1-1, 1)
		m = math.Floor((lon0*(nlRlat1-1)-lon1*nlRlat1)/cpr17 + 0.5)
		dLon := 360.0 / ni
		lon = dLon * (float64(cprMod(int(m), int(ni))) + lon1/cpr17)
		lat = rlat1
	}

	if lon > 180 {
		lon -= 360
	}

	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0, false
	}

	return lat, lon, true
}

// cprModF is a floating point version of the always-positive modulo,
// used by the local (relative) CPR decoding algorithm.
func cprModF(a, b float64) float64 {
	res := math.Mod(a, b)
	if res < 0 {
		res += b
	}
	return res
}

// decodeCPRRelative decodes a position from a single CPR message using a
// known receiver (or otherwise nearby) reference position. This is the
// standard "local CPR decoding" algorithm from 1090-WP-9-14 / RTCA
// DO-260B §2.4.3.8.5, matching the reference implementation used by
// pyModeS and dump1090's surface-position decoder.
// Returns (latitude, longitude, ok).
func decodeCPRRelative(latEnc, lonEnc int, isOdd bool, refLat, refLon float64) (lat, lon float64, ok bool) {
	const cpr17 = 131072.0

	dLat := 360.0 / 60.0
	if isOdd {
		dLat = 360.0 / 59.0
	}

	latF := float64(latEnc) / cpr17
	j := math.Floor(refLat/dLat) + math.Floor(0.5+cprModF(refLat, dLat)/dLat-latF)
	lat = dLat * (j + latF)
	if lat >= 270 {
		lat -= 360
	}

	nlLat := nl(lat)
	ni := nlLat
	if isOdd {
		ni = nlLat - 1
	}
	if ni < 1 {
		ni = 1
	}

	dLon := 360.0 / ni
	lonF := float64(lonEnc) / cpr17
	m := math.Floor(refLon/dLon) + math.Floor(0.5+cprModF(refLon, dLon)/dLon-lonF)
	lon = dLon * (m + lonF)
	if lon >= 180 {
		lon -= 360
	}

	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0, false
	}

	return lat, lon, true
}
