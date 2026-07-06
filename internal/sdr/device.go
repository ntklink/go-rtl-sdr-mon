package sdr

import (
	"fmt"
	"sync"
)

// SDRDevice is the interface that all SDR hardware implementations must satisfy.
// This abstraction allows supporting multiple device types (RTL-SDR, HackRF, Airspy, etc.)
// behind a unified API.
type SDRDevice interface {
	// Info returns device information.
	Info() (DeviceInfo, error)

	// Samples returns the channel that delivers IQ sample blocks.
	Samples() <-chan []complex128

	// Start begins async sample reading. Blocks the calling goroutine until Stop.
	Start() error
	// Stop stops sample reading.
	Stop()
	// Close closes the device and releases all resources.
	Close()

	// Frequency
	SetCenterFreq(freq uint32) error
	GetCenterFreq() uint32

	// Sample rate
	SetSampleRate(rate uint32) error
	GetSampleRate() uint32

	// Gain
	SetAutoGain(auto bool) error
	SetGain(gain int) error // tenths of dB (e.g., 248 = 24.8 dB)
	GetGain() int
	IsAutoGain() bool

	// Frequency correction
	SetFreqCorrection(ppm int) error

	// Bandwidth (0 = auto)
	SetBandwidth(bw uint32) error

	// Bias tee
	SetBiasTee(on bool) error
}

// DeviceInfo holds information about an opened device.
type DeviceInfo struct {
	ID           string `json:"id"`           // unique device identifier (e.g., "rtlsdr-0")
	Driver       string `json:"driver"`       // driver name: "rtlsdr", "hackrf", etc.
	Index        int    `json:"index"`        // driver-specific index
	Name         string `json:"name"`         // device name
	Manufacturer string `json:"manufacturer"` // USB manufacturer
	Product      string `json:"product"`      // USB product
	Serial       string `json:"serial"`       // USB serial number
	TunerType    string `json:"tuner_type"`   // tuner type name
	SampleRate   uint32 `json:"sample_rate"`  // current sample rate in Hz
	CenterFreq   uint32 `json:"center_freq"`  // current center frequency in Hz
	Gains        []int  `json:"gains"`        // supported gain values in tenths of dB
	Active       bool   `json:"active"`       // whether this is the currently active device
}

// DeviceDescriptor describes a device available for opening (not yet opened).
type DeviceDescriptor struct {
	ID           string `json:"id"`           // unique identifier (e.g., "rtlsdr-0")
	Driver       string `json:"driver"`       // driver name
	Index        int    `json:"index"`        // driver-specific index
	Name         string `json:"name"`         // device name
	Manufacturer string `json:"manufacturer"` // USB manufacturer string
	Product      string `json:"product"`      // USB product string
	Serial       string `json:"serial"`       // USB serial number
}

// DeviceEnumerator is implemented by each driver to enumerate available devices.
type DeviceEnumerator interface {
	DriverName() string
	Enumerate() []DeviceDescriptor
	Open(index int, sampleRate, centerFreq uint32) (SDRDevice, error)
}

// DeviceManager manages multiple SDR devices of potentially different types.
// It supports device enumeration, opening, and selecting an active device.
type DeviceManager struct {
	mu          sync.RWMutex
	enumerators []DeviceEnumerator
	devices     map[string]SDRDevice // opened devices by ID
	activeID    string
}

// NewDeviceManager creates a new DeviceManager with the given enumerators.
func NewDeviceManager(enumerators ...DeviceEnumerator) *DeviceManager {
	return &DeviceManager{
		enumerators: enumerators,
		devices:     make(map[string]SDRDevice),
	}
}

// Enumerate returns all available devices from all registered drivers.
func (m *DeviceManager) Enumerate() []DeviceDescriptor {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var all []DeviceDescriptor
	for _, e := range m.enumerators {
		descs := e.Enumerate()
		all = append(all, descs...)
	}
	return all
}

// Open opens a device by its ID and returns it.
// If the device is already open, returns the existing instance.
func (m *DeviceManager) Open(id string, sampleRate, centerFreq uint32) (SDRDevice, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Already open?
	if dev, ok := m.devices[id]; ok {
		return dev, nil
	}

	// Find the descriptor to determine which driver to use
	for _, e := range m.enumerators {
		for _, desc := range e.Enumerate() {
			if desc.ID == id {
				dev, err := e.Open(desc.Index, sampleRate, centerFreq)
				if err != nil {
					return nil, fmt.Errorf("open %s: %w", id, err)
				}
				m.devices[id] = dev
				if m.activeID == "" {
					m.activeID = id
				}
				return dev, nil
			}
		}
	}

	return nil, fmt.Errorf("device %s not found", id)
}

// SetActive sets the active device by ID.
func (m *DeviceManager) SetActive(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.devices[id]; !ok {
		return fmt.Errorf("device %s is not open", id)
	}
	m.activeID = id
	return nil
}

// Active returns the currently active device, or nil if none.
func (m *DeviceManager) Active() SDRDevice {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.activeID == "" {
		return nil
	}
	return m.devices[m.activeID]
}

// ActiveID returns the ID of the currently active device.
func (m *DeviceManager) ActiveID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeID
}

// Get returns an opened device by ID.
func (m *DeviceManager) Get(id string) (SDRDevice, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dev, ok := m.devices[id]
	return dev, ok
}

// CloseDevice closes a specific device by ID.
func (m *DeviceManager) CloseDevice(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if dev, ok := m.devices[id]; ok {
		dev.Close()
		delete(m.devices, id)
		if m.activeID == id {
			m.activeID = ""
			// Pick a new active device if available
			for k := range m.devices {
				m.activeID = k
				break
			}
		}
	}
}

// CloseAll closes all opened devices.
func (m *DeviceManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, dev := range m.devices {
		dev.Close()
	}
	m.devices = make(map[string]SDRDevice)
	m.activeID = ""
}

// ListOpenDevices returns info about all opened devices.
func (m *DeviceManager) ListOpenDevices() []DeviceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []DeviceInfo
	for id, dev := range m.devices {
		info, err := dev.Info()
		if err != nil {
			continue
		}
		info.ID = id
		info.Active = id == m.activeID
		list = append(list, info)
	}
	return list
}
