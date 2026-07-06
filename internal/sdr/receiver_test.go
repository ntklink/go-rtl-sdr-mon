package sdr

import (
	"sync"
	"testing"
)

// mockSource is a no-op SDRDevice for testing the receiver without hardware.
type mockSource struct {
	sampleRate uint32
	centerFreq uint32
}

func (m *mockSource) Info() (DeviceInfo, error)              { return DeviceInfo{}, nil }
func (m *mockSource) Samples() <-chan []complex128           { return nil }
func (m *mockSource) Start() error                           { return nil }
func (m *mockSource) Stop()                                  {}
func (m *mockSource) Close()                                 {}
func (m *mockSource) SetCenterFreq(f uint32) error           { m.centerFreq = f; return nil }
func (m *mockSource) GetCenterFreq() uint32                  { return m.centerFreq }
func (m *mockSource) SetSampleRate(r uint32) error           { m.sampleRate = r; return nil }
func (m *mockSource) GetSampleRate() uint32                  { return m.sampleRate }
func (m *mockSource) SetAutoGain(bool) error                 { return nil }
func (m *mockSource) SetGain(int) error                      { return nil }
func (m *mockSource) GetGain() int                           { return 0 }
func (m *mockSource) IsAutoGain() bool                       { return false }
func (m *mockSource) SetFreqCorrection(int) error            { return nil }
func (m *mockSource) SetBandwidth(uint32) error              { return nil }
func (m *mockSource) SetBiasTee(bool) error                  { return nil }

// TestGetSpectrumSizeConcurrent exercises the data race between
// SetFFTSize (reassigns r.spectrum under r.mu) and GetSpectrumSize
// (reads r.spectrum). Run with -race.
func TestGetSpectrumSizeConcurrent(t *testing.T) {
	cfg := DefaultReceiverConfig()
	r := NewReceiver(&mockSource{sampleRate: cfg.SampleRate, centerFreq: cfg.CenterFreq}, cfg)

	sizes := []int{1024, 2048, 4096, 8192}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			r.SetFFTSize(sizes[i%len(sizes)])
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = r.GetSpectrumSize()
		}
	}()
	wg.Wait()
}
