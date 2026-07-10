package adsb

import (
	"math"
	"sort"
	"sync"
	"time"
)

// Tracker maintains a list of tracked aircraft, merging messages from
// the same ICAO address into a single Aircraft record.
type Tracker struct {
	mu       sync.RWMutex
	aircraft map[string]*Aircraft // keyed by ICAO hex, active aircraft
	history  map[string]*Aircraft // keyed by ICAO hex, all-time history (never auto-deleted)

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
		history:     make(map[string]*Aircraft),
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

	// Also update the history record (a separate copy so that
	// Cleanup deleting from t.aircraft doesn't affect history).
	hist, ok := t.history[msg.ICAO]
	if !ok {
		hist = &Aircraft{ICAO: msg.ICAO}
		t.history[msg.ICAO] = hist
	}

	aircraft.LastSeen = time.Now().UnixMilli()
	aircraft.MessageCount++
	aircraft.TypeCode = msg.TypeCode

	hist.LastSeen = aircraft.LastSeen
	hist.MessageCount = aircraft.MessageCount
	hist.TypeCode = aircraft.TypeCode

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
		if !math.IsNaN(speed) {
			aircraft.Speed = speed
		}
		if !math.IsNaN(track) {
			aircraft.Track = track
		}
		if !math.IsNaN(vRate) {
			aircraft.VerticalRate = int(vRate)
		}
	}

	// Sync all decoded fields to the history record.
	hist.Callsign = aircraft.Callsign
	hist.Latitude = aircraft.Latitude
	hist.Longitude = aircraft.Longitude
	hist.Altitude = aircraft.Altitude
	hist.Speed = aircraft.Speed
	hist.Track = aircraft.Track
	hist.VerticalRate = aircraft.VerticalRate
	hist.Squawk = aircraft.Squawk
	hist.OnGround = aircraft.OnGround
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
			// Global CPR decoding. Use whichever message arrived most
			// recently to pick the longitude zone (matches dump1090).
			useEven := evenTime.After(oddTime)
			lat, lon, ok := decodeCPRGlobal(evenLat, evenLon, oddLat, oddLon, useEven)
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

// GetAircraft returns a snapshot of all actively tracked aircraft.
// An aircraft is considered active if it has been seen in the last 120 seconds.
func (t *Tracker) GetAircraft() []Aircraft {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]Aircraft, 0, len(t.aircraft))
	for _, a := range t.aircraft {
		// Skip aircraft not seen in the last 120 seconds
		if time.Since(time.UnixMilli(a.LastSeen)) > 120*time.Second {
			continue
		}
		result = append(result, *a)
	}
	return result
}

// GetHistory returns a snapshot of all aircraft ever tracked, sorted by
// LastSeen descending (most recent first).  History records are never
// automatically deleted and persist across demod mode switches.
func (t *Tracker) GetHistory() []Aircraft {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]Aircraft, 0, len(t.history))
	for _, a := range t.history {
		result = append(result, *a)
	}
	// Sort by LastSeen descending (most recent first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastSeen > result[j].LastSeen
	})
	return result
}

// GetAllAircraft returns both active and historical aircraft. Active
// aircraft come first (sorted by callsign), followed by inactive history-only
// entries.  Each entry has a fresh boolean indicating active status.
func (t *Tracker) GetAllAircraft() []Aircraft {
	t.mu.RLock()
	defer t.mu.RUnlock()

	seen := make(map[string]bool, len(t.aircraft))
	result := make([]Aircraft, 0, len(t.history))

	// Active aircraft first (seen in last 120 seconds)
	for icao, a := range t.aircraft {
		if time.Since(time.UnixMilli(a.LastSeen)) <= 120*time.Second {
			result = append(result, *a)
			seen[icao] = true
		}
	}

	// Then history-only entries (not in active set)
	for icao, a := range t.history {
		if !seen[icao] {
			result = append(result, *a)
		}
	}

	return result
}

// Count returns the number of actively tracked aircraft (seen in last 120 seconds).
func (t *Tracker) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	count := 0
	for _, a := range t.aircraft {
		if time.Since(time.UnixMilli(a.LastSeen)) <= 120*time.Second {
			count++
		}
	}
	return count
}

// HistoryCount returns the total number of aircraft ever tracked.
func (t *Tracker) HistoryCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.history)
}

// Cleanup removes stale aircraft from the active map (not seen in the
// last 5 minutes).  History records are never deleted.
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
