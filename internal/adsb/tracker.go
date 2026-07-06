package adsb

import (
	"sync"
	"time"
)

// Tracker maintains a list of tracked aircraft, merging messages from
// the same ICAO address into a single Aircraft record.
type Tracker struct {
	mu       sync.RWMutex
	aircraft map[string]*Aircraft // keyed by ICAO hex

	// CPR storage for position decoding
	cprEvenLat  map[string]int
	cprEvenLon  map[string]int
	cprOddLat   map[string]int
	cprOddLon   map[string]int
	cprEvenTime map[string]time.Time
	cprOddTime  map[string]time.Time

	// Receiver position for relative CPR decoding
	refLat    float64
	refLon    float64
	hasRefPos bool
}

// NewTracker creates a new aircraft tracker.
func NewTracker() *Tracker {
	return &Tracker{
		aircraft:    make(map[string]*Aircraft),
		cprEvenLat:  make(map[string]int),
		cprEvenLon:  make(map[string]int),
		cprOddLat:   make(map[string]int),
		cprOddLon:   make(map[string]int),
		cprEvenTime: make(map[string]time.Time),
		cprOddTime:  make(map[string]time.Time),
	}
}

// SetReceiverPosition sets the receiver's position for relative CPR decoding.
// Safe for concurrent use (refLat/refLon are read under t.mu in decodePosition).
func (t *Tracker) SetReceiverPosition(lat, lon float64) {
	t.mu.Lock()
	t.refLat = lat
	t.refLon = lon
	t.hasRefPos = true
	t.mu.Unlock()
}

// ProcessMessage processes a decoded ADS-B message and updates aircraft state.
func (t *Tracker) ProcessMessage(msg *Message) {
	if msg == nil || msg.ICAO == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	aircraft, ok := t.aircraft[msg.ICAO]
	if !ok {
		aircraft = &Aircraft{
			ICAO: msg.ICAO,
		}
		t.aircraft[msg.ICAO] = aircraft
	}

	aircraft.LastSeen = time.Now().UnixMilli()
	aircraft.MessageCount++
	aircraft.TypeCode = msg.TypeCode

	// Decode based on type code
	tc := msg.TypeCode

	// Type codes 1-4: Aircraft identification
	if tc >= 1 && tc <= 4 {
		callsign := decodeCallsign(msg.ME)
		if callsign != "" {
			aircraft.Callsign = callsign
		}
	}

	// Type codes 9-18: Airborne position
	if tc >= 9 && tc <= 18 {
		alt := decodeAltitude(msg.ME)
		if alt > 0 {
			aircraft.Altitude = alt
		}

		latEnc, lonEnc, isOdd := decodeAirbornePosition(msg.ME)

		// Store CPR data
		if isOdd {
			t.cprOddLat[msg.ICAO] = latEnc
			t.cprOddLon[msg.ICAO] = lonEnc
			t.cprOddTime[msg.ICAO] = time.Now()
		} else {
			t.cprEvenLat[msg.ICAO] = latEnc
			t.cprEvenLon[msg.ICAO] = lonEnc
			t.cprEvenTime[msg.ICAO] = time.Now()
		}

		// Try to decode position
		t.decodePosition(aircraft, msg.ICAO)
	}

	// Type code 19: Airborne velocity
	if tc == 19 {
		speed, track, vRate := decodeVelocity(msg.ME)
		if speed > 0 {
			aircraft.Speed = speed
		}
		if track > 0 {
			aircraft.Track = track
		}
		aircraft.VerticalRate = int(vRate)
	}
}

// decodePosition attempts to decode aircraft position from stored CPR data.
func (t *Tracker) decodePosition(aircraft *Aircraft, icao string) {
	evenLat, hasEven := t.cprEvenLat[icao]
	evenLon, hasEvenLon := t.cprEvenLon[icao]
	oddLat, hasOdd := t.cprOddLat[icao]
	oddLon, hasOddLon := t.cprOddLon[icao]

	if hasEven && hasOdd && hasEvenLon && hasOddLon {
		// Check that both messages are within 10 seconds
		evenTime := t.cprEvenTime[icao]
		oddTime := t.cprOddTime[icao]
		if time.Since(evenTime) < 10*time.Second && time.Since(oddTime) < 10*time.Second {
			// Global CPR decoding
			lat, lon, ok := decodeCPRGlobal(evenLat, evenLon, oddLat, oddLon)
			if ok {
				aircraft.Latitude = lat
				aircraft.Longitude = lon
			}
		}
	} else if hasEven && t.hasRefPos {
		// Relative CPR decoding using receiver position
		lat, lon, ok := decodeCPRRelative(evenLat, evenLon, false, t.refLat, t.refLon)
		if ok {
			aircraft.Latitude = lat
			aircraft.Longitude = lon
		}
	} else if hasOdd && t.hasRefPos {
		lat, lon, ok := decodeCPRRelative(oddLat, oddLon, true, t.refLat, t.refLon)
		if ok {
			aircraft.Latitude = lat
			aircraft.Longitude = lon
		}
	}
}

// GetAircraft returns a snapshot of all tracked aircraft.
func (t *Tracker) GetAircraft() []Aircraft {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]Aircraft, 0, len(t.aircraft))
	for _, a := range t.aircraft {
		// Skip aircraft not seen in the last 60 seconds
		if time.Since(time.UnixMilli(a.LastSeen)) > 60*time.Second {
			continue
		}
		result = append(result, *a)
	}
	return result
}

// Count returns the number of tracked aircraft.
func (t *Tracker) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	count := 0
	for _, a := range t.aircraft {
		if time.Since(time.UnixMilli(a.LastSeen)) <= 60*time.Second {
			count++
		}
	}
	return count
}

// Cleanup removes stale aircraft (not seen in the last 5 minutes).
func (t *Tracker) Cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for icao, a := range t.aircraft {
		if time.Since(time.UnixMilli(a.LastSeen)) > 5*time.Minute {
			delete(t.aircraft, icao)
			delete(t.cprEvenLat, icao)
			delete(t.cprEvenLon, icao)
			delete(t.cprOddLat, icao)
			delete(t.cprOddLon, icao)
			delete(t.cprEvenTime, icao)
			delete(t.cprOddTime, icao)
		}
	}
}
