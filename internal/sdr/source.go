package sdr

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	rtlsdr "github.com/ntklink/go-rtl-sdr"
)

// RTLSDRSource wraps an RTL-SDR device and provides a stream of complex128 samples.
// Implements the SDRDevice interface.
type RTLSDRSource struct {
	dev            *rtlsdr.Device
	index          int // device index for re-open
	sampleRate     uint32
	centerFreq     uint32
	freqCorrection int
	autoGain       atomic.Bool
	gain           int // tenths of dB
	bandwidth      uint32

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}

	// bufferSize is the async read buffer size in bytes (0 = default).
	bufferSize uint32

	// sampleCh delivers complex128 IQ sample blocks to the consumer.
	sampleCh chan []complex128
}

// OpenRTLSDR opens the RTL-SDR device at the given index and configures defaults.
func OpenRTLSDR(index int, sampleRate, centerFreq uint32) (*RTLSDRSource, error) {
	count := rtlsdr.GetDeviceCount()
	if count == 0 {
		return nil, fmt.Errorf("no RTL-SDR devices found")
	}
	if index < 0 || index >= count {
		return nil, fmt.Errorf("device index %d out of range (count=%d)", index, count)
	}

	dev, err := rtlsdr.Open(index)
	if err != nil {
		return nil, fmt.Errorf("open device %d: %w", index, err)
	}

	s := &RTLSDRSource{
		dev:        dev,
		index:      index,
		sampleRate: sampleRate,
		centerFreq: centerFreq,
		bufferSize: 147456, // 9×16384 bytes → ~24.4 fps at 1.8 MHz
		stopCh:     make(chan struct{}),
		sampleCh:   make(chan []complex128, 4),
	}

	// Configure defaults
	if err := dev.SetSampleRate(sampleRate); err != nil {
		_ = dev.Close()
		return nil, fmt.Errorf("set sample rate: %w", err)
	}
	if err := dev.SetCenterFreq(centerFreq); err != nil {
		_ = dev.Close()
		return nil, fmt.Errorf("set center freq: %w", err)
	}
	// Auto gain by default
	if err := dev.SetTunerGainMode(false); err != nil {
		_ = dev.Close()
		return nil, fmt.Errorf("set gain mode: %w", err)
	}
	s.autoGain.Store(true)

	// Set auto bandwidth
	if err := dev.SetTunerBandwidth(0); err != nil {
		log.Printf("warning: set bandwidth: %v", err)
	}

	return s, nil
}

// Info returns device information.
func (s *RTLSDRSource) Info() (DeviceInfo, error) {
	usb, err := s.dev.GetUSBStrings()
	if err != nil {
		usb = rtlsdr.USBStrings{}
	}
	gains, _ := s.dev.GetTunerGains()
	return DeviceInfo{
		Driver:       "rtlsdr",
		Index:        s.index,
		Name:         rtlsdr.GetDeviceName(s.index),
		Manufacturer: usb.Manufacturer,
		Product:      usb.Product,
		Serial:       usb.Serial,
		TunerType:    s.dev.GetTunerType().String(),
		SampleRate:   s.dev.GetSampleRate(),
		CenterFreq:   s.dev.GetCenterFreq(),
		Gains:        gains,
	}, nil
}

// Samples returns the channel that delivers IQ sample blocks.
func (s *RTLSDRSource) Samples() <-chan []complex128 {
	return s.sampleCh
}

// Start begins async sample reading. Blocks the calling goroutine until Stop.
func (s *RTLSDRSource) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("already running")
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	if err := s.dev.ResetBuffer(); err != nil {
		return fmt.Errorf("reset buffer: %w", err)
	}

	// Use async reading with custom buffer size for ~25fps FFT rate
	bufLen := s.bufferSize
	if bufLen == 0 {
		bufLen = 147456 // default: 9×16384
	}
	err := s.dev.ReadAsync(func(data []byte) {
		samples := bytesToComplex(data)
		select {
		case s.sampleCh <- samples:
		case <-s.stopCh:
		}
	}, 0, bufLen)

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	return err
}

// Stop stops sample reading.
func (s *RTLSDRSource) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	close(s.stopCh)
	s.running = false
	_ = s.dev.CancelAsync()
}

// Close closes the device.
func (s *RTLSDRSource) Close() {
	s.Stop()
	if s.dev != nil {
		_ = s.dev.Close()
		s.dev = nil
	}
}

// SetCenterFreq sets the center frequency in Hz.
func (s *RTLSDRSource) SetCenterFreq(freq uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.centerFreq = freq
	return s.dev.SetCenterFreq(freq)
}

// GetCenterFreq returns the current center frequency in Hz.
func (s *RTLSDRSource) GetCenterFreq() uint32 {
	return s.dev.GetCenterFreq()
}

// SetFreqCorrection sets the frequency correction in ppm.
func (s *RTLSDRSource) SetFreqCorrection(ppm int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.freqCorrection = ppm
	return s.dev.SetFreqCorrection(ppm)
}

// SetSampleRate sets the sample rate in Hz.
func (s *RTLSDRSource) SetSampleRate(rate uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sampleRate = rate
	return s.dev.SetSampleRate(rate)
}

// GetSampleRate returns the current sample rate in Hz.
func (s *RTLSDRSource) GetSampleRate() uint32 {
	return s.dev.GetSampleRate()
}

// SetAutoGain enables or disables automatic gain.
func (s *RTLSDRSource) SetAutoGain(auto bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autoGain.Store(auto)
	mode := !auto // false=auto gain, true=manual gain
	if err := s.dev.SetTunerGainMode(mode); err != nil {
		return err
	}
	if auto {
		// RTL AGC on for auto mode
		_ = s.dev.SetAGCMode(true)
	} else {
		_ = s.dev.SetAGCMode(false)
		if s.gain != 0 {
			return s.dev.SetTunerGain(s.gain)
		}
	}
	return nil
}

// SetGain sets the manual gain in tenths of dB (e.g., 115 = 11.5 dB).
func (s *RTLSDRSource) SetGain(gain int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gain = gain
	return s.dev.SetTunerGain(gain)
}

// GetGain returns the current gain in tenths of dB.
func (s *RTLSDRSource) GetGain() int {
	return s.dev.GetTunerGain()
}

// IsAutoGain returns whether auto gain is enabled.
func (s *RTLSDRSource) IsAutoGain() bool {
	return s.autoGain.Load()
}

// SetBandwidth sets the tuner bandwidth in Hz (0 = auto).
func (s *RTLSDRSource) SetBandwidth(bw uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bandwidth = bw
	return s.dev.SetTunerBandwidth(bw)
}

// SetBiasTee enables or disables the bias-T.
func (s *RTLSDRSource) SetBiasTee(on bool) error {
	return s.dev.SetBiasTee(on)
}

// bytesToComplex converts RTL-SDR 8-bit unsigned IQ bytes to complex128 samples.
// Each pair of bytes (I, Q) becomes one complex sample.
// Values are centered around 0 and normalized to [-1, 1].
func bytesToComplex(data []byte) []complex128 {
	n := len(data) / 2
	samples := make([]complex128, n)
	for i := 0; i < n; i++ {
		re := (float64(data[2*i]) - 127.5) / 127.5
		im := (float64(data[2*i+1]) - 127.5) / 127.5
		samples[i] = complex(re, im)
	}
	return samples
}

// Compile-time interface checks.
var _ SDRDevice = (*RTLSDRSource)(nil)

// RTLSDREnumerator implements DeviceEnumerator for RTL-SDR devices.
type RTLSDREnumerator struct{}

// DriverName returns the driver name.
func (RTLSDREnumerator) DriverName() string { return "rtlsdr" }

// Enumerate returns all available RTL-SDR devices.
func (RTLSDREnumerator) Enumerate() []DeviceDescriptor {
	count := rtlsdr.GetDeviceCount()
	descs := make([]DeviceDescriptor, 0, count)
	for i := 0; i < count; i++ {
		usb, err := rtlsdr.GetDeviceUSBStrings(i)
		if err != nil {
			usb = rtlsdr.USBStrings{}
		}
		serial := usb.Serial
		if serial == "" {
			serial = fmt.Sprintf("idx%d", i)
		}
		descs = append(descs, DeviceDescriptor{
			ID:           fmt.Sprintf("rtlsdr-%s", serial),
			Driver:       "rtlsdr",
			Index:        i,
			Name:         rtlsdr.GetDeviceName(i),
			Manufacturer: usb.Manufacturer,
			Product:      usb.Product,
			Serial:       usb.Serial,
		})
	}
	return descs
}

// Open opens an RTL-SDR device at the given index.
func (RTLSDREnumerator) Open(index int, sampleRate, centerFreq uint32) (SDRDevice, error) {
	return OpenRTLSDR(index, sampleRate, centerFreq)
}
