package adsb

import (
	"sync"
	"testing"
)

// TestTrackerReceiverPositionConcurrent exercises the data race between
// SetReceiverPosition (HTTP handler goroutine) and ProcessMessage ->
// decodePosition (processLoop goroutine), which both touch refLat/refLon.
// Run with -race to verify the lock fix.
func TestTrackerReceiverPositionConcurrent(t *testing.T) {
	tr := NewTracker()

	// A TC=9 airborne-position message so ProcessMessage reaches decodePosition,
	// which reads refLat/refLon/hasRefPos.
	msg := &Message{
		ICAO:     "ABCDEF",
		TypeCode: 9,
		ME:       make([]byte, 7),
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer: HTTP handler setting receiver position (including lat=0, which
	// previously broke the `refLat != 0` sentinel).
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			tr.SetReceiverPosition(float64(i%180)-90, float64(i%360)-180)
		}
	}()

	// Reader: processLoop feeding messages, which reads the receiver position.
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			tr.ProcessMessage(msg)
		}
	}()

	wg.Wait()

	// Final state should be usable.
	_ = tr.GetAircraft()
}
