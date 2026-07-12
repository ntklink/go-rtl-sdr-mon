package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"log"
	"math"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/ntklink/GoEther-SDR/internal/lrpt"
	"github.com/ntklink/GoEther-SDR/internal/sdr"
)

// --- Utility functions (migrated from console.go) ---

// EnsureTLSCert checks for cert.pem and key.pem in the executable's directory.
// If either is missing, a self-signed ECDSA certificate is generated automatically.
// Returns the absolute paths to the certificate and key files.
func EnsureTLSCert() (certFile, keyFile string, err error) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	dir := filepath.Dir(exe)
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")

	// Both files exist — nothing to do
	if fileExists(certFile) && fileExists(keyFile) {
		return certFile, keyFile, nil
	}

	log.Printf("Generating self-signed TLS certificate...")

	// Generate ECDSA P-256 private key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}

	// Collect IP addresses for SAN (localhost + all network interfaces)
	var ipAddrs []net.IP
	ipAddrs = append(ipAddrs, net.IPv4(127, 0, 0, 1), net.IPv6loopback)
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip != nil && !ip.IsUnspecified() {
					ipAddrs = append(ipAddrs, ip)
				}
			}
		}
	}

	// Build certificate template
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "goether-sdr",
			Organization: []string{"goether-sdr"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           ipAddrs,
	}

	// Self-sign the certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}

	// Encode certificate to PEM
	certOut, err := os.Create(certFile)
	if err != nil {
		return "", "", err
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		_ = certOut.Close()
		return "", "", err
	}
	_ = certOut.Close()

	// Encode private key to PEM
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return "", "", err
	}
	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", "", err
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		_ = keyOut.Close()
		return "", "", err
	}
	_ = keyOut.Close()

	log.Printf("TLS certificate written to %s", certFile)
	log.Printf("TLS private key written to %s", keyFile)

	return certFile, keyFile, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// demodOptions returns the list of available demodulator names (matches gqrx order).
func demodOptions() []string {
	return []string{
		"OFF", "Raw I/Q", "AM", "AM-Sync", "LSB", "USB",
		"CW-L", "CW-U", "NFM", "WFM", "WFM-Stereo", "WFM-OIRT", "ADS-B", "LRPT",
	}
}

// floatToInt16 converts a float32 sample to int16 with clamping.
func floatToInt16(v float32) int16 {
	if v > 1 {
		v = 1
	} else if v < -1 {
		v = -1
	}
	return int16(v * 32767)
}

// Server holds the HTTP server state and references to the SDR receiver.
type Server struct {
	receiver *sdr.Receiver
	dm       *sdr.DeviceManager
}

// NewServer creates a new Server.
func NewServer(dm *sdr.DeviceManager, receiver *sdr.Receiver) *Server {
	return &Server{
		receiver: receiver,
		dm:       dm,
	}
}

// RegisterRoutes registers all API routes on the Echo instance.
func (s *Server) RegisterRoutes(e *echo.Echo) {
	api := e.Group("/api")

	// Device info
	api.GET("/device", s.handleDeviceInfo)
	api.GET("/devices", s.handleListDevices)
	api.POST("/device/select", s.handleSelectDevice)

	// Receiver control
	api.GET("/status", s.handleStatus)
	api.GET("/demods", s.handleDemods)
	api.POST("/frequency", s.handleSetFrequency)
	api.POST("/demod", s.handleSetDemod)
	api.POST("/filter", s.handleSetFilter)
	api.POST("/filter-offset", s.handleSetFilterOffset)
	api.POST("/squelch", s.handleSetSquelch)
	api.POST("/agc", s.handleSetAGC)
	api.POST("/gain", s.handleSetGain)
	api.POST("/auto-gain", s.handleSetAutoGain)
	api.POST("/freq-correction", s.handleSetFreqCorrection)
	api.POST("/spectrum-avg", s.handleSetSpectrumAvg)
	api.POST("/fft-size", s.handleSetFFTSize)
	api.POST("/fft-rate", s.handleSetFFTRate)
	api.POST("/fft-max-hold", s.handleSetFFTMaxHold)
	api.POST("/agc-preset", s.handleSetAGCPreset)
	api.POST("/cw-offset", s.handleSetCWOffset)
	api.POST("/filter-shape", s.handleSetFilterShape)
	api.POST("/filter-preset", s.handleSetFilterPreset)

	// WebSocket endpoints
	api.GET("/ws/fft", s.handleWSFFT)
	api.GET("/ws/audio", s.handleWSAudio)
	api.GET("/ws/status", s.handleWSStatus)
	api.GET("/ws/aircraft", s.handleWSAircraft)
	api.POST("/receiver-position", s.handleSetReceiverPosition)
	api.GET("/aircraft", s.handleGetAircraft)
	api.GET("/aircraft/history", s.handleGetAircraftHistory)
	api.GET("/aircraft/all", s.handleGetAllAircraft)
	api.GET("/adsb-stats", s.handleADSBStats)

	// Meteor-M LRPT
	api.GET("/ws/lrpt", s.handleWSLRPT)
	api.GET("/lrpt/satellites", s.handleGetLRPTSatellites)
	api.GET("/lrpt-stats", s.handleLRPTStats)
	api.POST("/lrpt-reset", s.handleResetLRPT)
}

// --- REST API Handlers ---

func (s *Server) handleDeviceInfo(c echo.Context) error {
	dev := s.dm.Active()
	if dev == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "no active device"})
	}
	info, err := dev.Info()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	info.ID = s.dm.ActiveID()
	info.Active = true
	return c.JSON(http.StatusOK, info)
}

func (s *Server) handleListDevices(c echo.Context) error {
	available := s.dm.Enumerate()
	open := s.dm.ListOpenDevices()
	activeID := s.dm.ActiveID()

	// Merge: mark which devices are open and which is active
	openMap := make(map[string]sdr.DeviceInfo)
	for _, o := range open {
		openMap[o.ID] = o
	}

	type DeviceListItem struct {
		sdr.DeviceDescriptor
		Open   bool `json:"open"`
		Active bool `json:"active"`
	}

	list := make([]DeviceListItem, 0, len(available))
	for _, desc := range available {
		item := DeviceListItem{DeviceDescriptor: desc}
		if info, ok := openMap[desc.ID]; ok {
			item.Open = true
			item.Active = info.Active
		}
		item.Active = item.Active || desc.ID == activeID
		list = append(list, item)
	}

	return c.JSON(http.StatusOK, map[string]any{"devices": list})
}

type selectDeviceRequest struct {
	ID string `json:"id"`
}

func (s *Server) handleSelectDevice(c echo.Context) error {
	var req selectDeviceRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if req.ID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing device id"})
	}

	// Open the device if it isn't open yet.
	dev, ok := s.dm.Get(req.ID)
	if !ok {
		config := s.receiver.GetConfig()
		d, err := s.dm.Open(req.ID, config.SampleRate, config.CenterFreq)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		dev = d
		// Apply current receiver settings to the newly opened device.
		if config.FreqCorrection != 0 {
			_ = dev.SetFreqCorrection(config.FreqCorrection)
		}
		if !config.AutoGain {
			_ = dev.SetAutoGain(false)
			if config.Gain != 0 {
				_ = dev.SetGain(config.Gain)
			}
		}
	}

	// Already the receiver's active source? Nothing to switch.
	if cur := s.receiver.Source(); cur != nil && cur == dev {
		_ = s.dm.SetActive(req.ID)
		return c.JSON(http.StatusOK, map[string]string{"id": req.ID, "status": "active"})
	}

	// Hand the new source to the receiver: it stops the old source's stream,
	// rebuilds rate-dependent DSP, starts the new source, and restarts
	// processing. Without this the receiver kept reading the old device.
	s.receiver.ReplaceSource(dev)
	_ = s.dm.SetActive(req.ID)
	return c.JSON(http.StatusOK, map[string]string{"id": req.ID, "status": "opened"})
}

func (s *Server) handleStatus(c echo.Context) error {
	status := s.receiver.GetStatus()
	config := s.receiver.GetConfig()
	return c.JSON(http.StatusOK, map[string]any{
		"status":    status,
		"config":    config,
		"auto_gain": config.AutoGain,
		"fft_size":  s.receiver.GetSpectrumSize(),
	})
}

func (s *Server) handleDemods(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{"demods": demodOptions()})
}

type freqRequest struct {
	Frequency uint32 `json:"frequency"`
}

func (s *Server) handleSetFrequency(c echo.Context) error {
	var req freqRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if err := s.receiver.SetCenterFreq(req.Frequency); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"frequency": req.Frequency})
}

type demodRequest struct {
	Demod string `json:"demod"`
}

func (s *Server) handleSetDemod(c echo.Context) error {
	var req demodRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	var dt sdr.DemodType
	switch req.Demod {
	case "OFF", "off":
		dt = sdr.DemodOff
	case "Raw I/Q", "Raw", "raw":
		dt = sdr.DemodRaw
	case "AM", "am":
		dt = sdr.DemodAM
	case "AM-Sync", "am-sync", "AMSync":
		dt = sdr.DemodAMSync
	case "LSB", "lsb":
		dt = sdr.DemodLSB
	case "USB", "usb":
		dt = sdr.DemodUSB
	case "CW-L", "cw-l", "CWL":
		dt = sdr.DemodCWL
	case "CW-U", "cw-u", "CWU":
		dt = sdr.DemodCWU
	case "NFM", "nfm", "FM", "fm":
		dt = sdr.DemodNFM
	case "WFM", "wfm":
		dt = sdr.DemodWFM
	case "WFM-Stereo", "wfm-stereo", "WFMS":
		dt = sdr.DemodWFMStereo
	case "WFM-OIRT", "wfm-oirt", "WFMO":
		dt = sdr.DemodWFMOirt
	case "ADS-B", "adsb", "ads-b", "ADS":
		dt = sdr.DemodADSB
	case "LRPT", "lrpt", "NOAA", "noaa":
		dt = sdr.DemodLRPT
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unknown demod type"})
	}

	s.receiver.SetDemod(dt)
	return c.JSON(http.StatusOK, map[string]string{"demod": dt.String()})
}

type filterRequest struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

func (s *Server) handleSetFilter(c echo.Context) error {
	var req filterRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	s.receiver.SetFilter(req.Low, req.High)
	return c.JSON(http.StatusOK, req)
}

type filterOffsetRequest struct {
	Offset float64 `json:"offset"`
}

func (s *Server) handleSetFilterOffset(c echo.Context) error {
	var req filterOffsetRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	s.receiver.SetFilterOffset(req.Offset)
	return c.JSON(http.StatusOK, req)
}

type squelchRequest struct {
	Level float64 `json:"level"`
}

func (s *Server) handleSetSquelch(c echo.Context) error {
	var req squelchRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	s.receiver.SetSquelch(req.Level)
	return c.JSON(http.StatusOK, req)
}

type agcRequest struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handleSetAGC(c echo.Context) error {
	var req agcRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	s.receiver.SetAGC(req.Enabled)
	return c.JSON(http.StatusOK, req)
}

type gainRequest struct {
	Gain int `json:"gain"` // tenths of dB
}

func (s *Server) handleSetGain(c echo.Context) error {
	var req gainRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if err := s.receiver.SetGain(req.Gain); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, req)
}

type autoGainRequest struct {
	Auto bool `json:"auto"`
}

func (s *Server) handleSetAutoGain(c echo.Context) error {
	var req autoGainRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if err := s.receiver.SetAutoGain(req.Auto); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, req)
}

type freqCorrectionRequest struct {
	PPM int `json:"ppm"`
}

func (s *Server) handleSetFreqCorrection(c echo.Context) error {
	var req freqCorrectionRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if err := s.receiver.SetFreqCorrection(req.PPM); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, req)
}

type spectrumAvgRequest struct {
	Avg float64 `json:"avg"`
}

func (s *Server) handleSetSpectrumAvg(c echo.Context) error {
	var req spectrumAvgRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	s.receiver.SetSpectrumAvg(req.Avg)
	return c.JSON(http.StatusOK, req)
}

type fftSizeRequest struct {
	Size int `json:"size"`
}

// fftSizeMin/Max bound the FFT size accepted from clients. The UI only ever
// offers 1024-16384; the range is kept wide enough for that plus headroom
// while still rejecting sizes large enough to exhaust memory on a Pi.
const (
	fftSizeMin = 256
	fftSizeMax = 32768
)

func (s *Server) handleSetFFTSize(c echo.Context) error {
	var req fftSizeRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if req.Size < fftSizeMin || req.Size > fftSizeMax {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "size out of range"})
	}
	s.receiver.SetFFTSize(req.Size)
	return c.JSON(http.StatusOK, req)
}

type fftRateRequest struct {
	Rate float64 `json:"rate"`
}

func (s *Server) handleSetFFTRate(c echo.Context) error {
	var req fftRateRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	s.receiver.SetFFTRate(req.Rate)
	return c.JSON(http.StatusOK, req)
}

type fftMaxHoldRequest struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handleSetFFTMaxHold(c echo.Context) error {
	var req fftMaxHoldRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	s.receiver.SetFFTMaxHold(req.Enabled)
	return c.JSON(http.StatusOK, req)
}

type agcPresetRequest struct {
	Preset string `json:"preset"`
}

func (s *Server) handleSetAGCPreset(c echo.Context) error {
	var req agcPresetRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	var preset sdr.AGCPreset
	switch req.Preset {
	case "off", "Off", "OFF":
		preset = sdr.AGCPresetOff
	case "slow", "Slow", "SLOW":
		preset = sdr.AGCPresetSlow
	case "medium", "Medium", "MEDIUM":
		preset = sdr.AGCPresetMedium
	case "fast", "Fast", "FAST":
		preset = sdr.AGCPresetFast
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unknown AGC preset"})
	}
	s.receiver.SetAGCPreset(preset)
	return c.JSON(http.StatusOK, map[string]string{"preset": preset.String()})
}

type cwOffsetRequest struct {
	Offset float64 `json:"offset"`
}

func (s *Server) handleSetCWOffset(c echo.Context) error {
	var req cwOffsetRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	s.receiver.SetCWOffset(req.Offset)
	return c.JSON(http.StatusOK, req)
}

type filterShapeRequest struct {
	Shape string `json:"shape"`
}

func (s *Server) handleSetFilterShape(c echo.Context) error {
	var req filterShapeRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	var shape int
	switch req.Shape {
	case "soft", "Soft", "SOFT":
		shape = sdr.FilterShapeSoft
	case "sharp", "Sharp", "SHARP":
		shape = sdr.FilterShapeSharp
	default:
		shape = sdr.FilterShapeNormal
	}
	s.receiver.SetFilterShape(shape)
	return c.JSON(http.StatusOK, map[string]string{"shape": req.Shape})
}

type filterPresetRequest struct {
	Preset string `json:"preset"`
}

func (s *Server) handleSetFilterPreset(c echo.Context) error {
	var req filterPresetRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	var preset int
	switch req.Preset {
	case "wide", "Wide", "WIDE":
		preset = sdr.FilterPresetWide
	case "narrow", "Narrow", "NARROW":
		preset = sdr.FilterPresetNarrow
	default:
		preset = sdr.FilterPresetNormal
	}
	s.receiver.SetFilterPreset(preset)
	return c.JSON(http.StatusOK, map[string]string{"preset": req.Preset})
}

// --- WebSocket Handlers ---

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // allow all origins for development
	},
}

// WebSocket keepalive tuning. Every handler installs a read deadline that
// only a client Pong (sent automatically in response to our Ping) or an
// actual client message extends; a connection that goes dark without a
// clean close (Wi-Fi drop, phone sleep) is therefore reaped after
// wsPongWait instead of leaking a subscriber forever.
const (
	wsWriteWait  = 10 * time.Second
	wsPongWait   = 60 * time.Second
	wsPingPeriod = 25 * time.Second // must be < wsPongWait
)

// startWSReadPump installs a read deadline + pong handler and starts a
// goroutine that drains incoming frames (processing pongs/close control
// frames along the way). On any read error — including a deadline timeout —
// it closes ws, which unblocks a writer that may be parked in the caller's
// main select loop. Callers keep their own `defer ws.Close()`; the extra
// call from here is harmless.
func startWSReadPump(ws *websocket.Conn) {
	_ = ws.SetReadDeadline(time.Now().Add(wsPongWait))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(wsPongWait))
	})
	go func() {
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				_ = ws.Close()
				return
			}
		}
	}()
}

// wsWriteMessage writes with a write deadline so a stalled client can't
// block the sending goroutine indefinitely.
func wsWriteMessage(ws *websocket.Conn, messageType int, data []byte) error {
	_ = ws.SetWriteDeadline(time.Now().Add(wsWriteWait))
	return ws.WriteMessage(messageType, data)
}

// wsWriteJSON is the JSON counterpart of wsWriteMessage.
func wsWriteJSON(ws *websocket.Conn, v any) error {
	_ = ws.SetWriteDeadline(time.Now().Add(wsWriteWait))
	return ws.WriteJSON(v)
}

// handleWSFFT streams FFT spectrum data over WebSocket.
// The ?bins= query parameter controls how many bins are sent (decimated by averaging).
// Default is 1024 bins to reduce bandwidth.
func (s *Server) handleWSFFT(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = ws.Close() }()
	startWSReadPump(ws)

	fftCh := s.receiver.SubscribeFFT()
	defer s.receiver.UnsubscribeFFT(fftCh)

	// Parse desired output bins from query parameter
	bins := 1024 // default: decimate to 1024 bins
	if b := c.QueryParam("bins"); b != "" {
		if v := parseIntDefault(b, 0); v > 0 {
			bins = v
		}
	}

	// Send header: 4 bytes FFT size + 4 bytes output bins
	size := s.receiver.GetSpectrumSize()
	header := make([]byte, 8)
	binary.LittleEndian.PutUint32(header[0:4], uint32(size))
	binary.LittleEndian.PutUint32(header[4:8], uint32(bins))
	if err := wsWriteMessage(ws, websocket.BinaryMessage, header); err != nil {
		return nil
	}

	// Keepalive ping (reusable ticker avoids accumulating timers per iteration)
	ticker := time.NewTicker(wsPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case data, ok := <-fftCh:
			if !ok {
				return nil
			}
			// Decimate if needed
			out := decimateFFT(data, bins)
			buf := make([]byte, len(out)*4)
			for i, v := range out {
				binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
			}
			if err := wsWriteMessage(ws, websocket.BinaryMessage, buf); err != nil {
				return nil
			}
		case <-ticker.C:
			// Keepalive
			if err := wsWriteMessage(ws, websocket.PingMessage, nil); err != nil {
				return nil
			}
		}
	}
}

// decimateFFT reduces the number of FFT bins by averaging groups of bins.
// If bins >= len(data), the original data is returned unchanged.
func decimateFFT(data []float32, bins int) []float32 {
	n := len(data)
	if bins <= 0 || bins >= n {
		return data
	}
	out := make([]float32, bins)
	groupSize := float64(n) / float64(bins)
	for i := 0; i < bins; i++ {
		start := int(float64(i) * groupSize)
		end := int(float64(i+1) * groupSize)
		if end > n {
			end = n
		}
		if start >= end {
			start = end - 1
			if start < 0 {
				start = 0
			}
		}
		var sum float64
		count := 0
		for j := start; j < end; j++ {
			sum += float64(data[j])
			count++
		}
		if count > 0 {
			out[i] = float32(sum / float64(count))
		}
	}
	return out
}

// parseIntDefault parses an integer string, returning the default on error.
func parseIntDefault(s string, def int) int {
	v := 0
	neg := false
	for i, c := range s {
		if i == 0 && c == '-' {
			neg = true
			continue
		}
		if c < '0' || c > '9' {
			return def
		}
		v = v*10 + int(c-'0')
	}
	if neg {
		v = -v
	}
	return v
}

// handleWSAudio streams audio PCM data over WebSocket.
func (s *Server) handleWSAudio(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = ws.Close() }()
	startWSReadPump(ws)

	audioCh := s.receiver.SubscribeAudio()
	defer s.receiver.UnsubscribeAudio(audioCh)
	var writeMu sync.Mutex

	ticker := time.NewTicker(wsPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			writeMu.Lock()
			err := wsWriteMessage(ws, websocket.PingMessage, nil)
			writeMu.Unlock()
			if err != nil {
				return nil
			}
			continue
		case audio, ok := <-audioCh:
			if !ok {
				return nil
			}
			if err := writeAudioFrame(ws, &writeMu, audio); err != nil {
				return nil
			}
		}
	}
}

// writeAudioFrame encodes and writes one audio block.
// Format: [1 byte: channels] [4 bytes: sample count] [samples...]
// Each sample is int16 (2 bytes). Mono: left only, Stereo: interleaved L,R,L,R...
func writeAudioFrame(ws *websocket.Conn, writeMu *sync.Mutex, audio sdr.AudioBlock) error {
	channels := 1
	if audio.Right != nil {
		channels = 2
	}

	// The L/R resamplers are independent and may produce blocks that differ
	// in length by a sample; use the shorter length to avoid out-of-range.
	numSamples := len(audio.Left)
	if channels == 2 && len(audio.Right) < numSamples {
		numSamples = len(audio.Right)
	}

	bufLen := 1 + 4 + numSamples*channels*2
	buf := make([]byte, bufLen)
	buf[0] = byte(channels)
	binary.LittleEndian.PutUint32(buf[1:5], uint32(numSamples))

	offset := 5
	if channels == 1 {
		for i := 0; i < numSamples; i++ {
			iv := floatToInt16(audio.Left[i])
			binary.LittleEndian.PutUint16(buf[offset:], uint16(iv))
			offset += 2
		}
	} else {
		for i := 0; i < numSamples; i++ {
			lv := floatToInt16(audio.Left[i])
			rv := floatToInt16(audio.Right[i])
			binary.LittleEndian.PutUint16(buf[offset:], uint16(lv))
			offset += 2
			binary.LittleEndian.PutUint16(buf[offset:], uint16(rv))
			offset += 2
		}
	}

	writeMu.Lock()
	defer writeMu.Unlock()
	return wsWriteMessage(ws, websocket.BinaryMessage, buf)
}

// handleWSStatus streams status updates over WebSocket.
func (s *Server) handleWSStatus(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = ws.Close() }()
	startWSReadPump(ws)

	statusCh := s.receiver.SubscribeStatus()
	defer s.receiver.UnsubscribeStatus(statusCh)

	// Send periodic full status for responsive UI and keepalive
	statusTicker := time.NewTicker(500 * time.Millisecond)
	defer statusTicker.Stop()
	pingTicker := time.NewTicker(wsPingPeriod)
	defer pingTicker.Stop()

	for {
		select {
		case status, ok := <-statusCh:
			if !ok {
				return nil
			}
			if err := wsWriteJSON(ws, status); err != nil {
				return nil
			}
		case <-statusTicker.C:
			status := s.receiver.GetStatus()
			if err := wsWriteJSON(ws, status); err != nil {
				return nil
			}
		case <-pingTicker.C:
			if err := wsWriteMessage(ws, websocket.PingMessage, nil); err != nil {
				return nil
			}
		}
	}
}

// handleWSAircraft streams ADS-B aircraft data over WebSocket.
func (s *Server) handleWSAircraft(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = ws.Close() }()
	startWSReadPump(ws)

	aircraftCh := s.receiver.SubscribeAircraft()
	defer s.receiver.UnsubscribeAircraft(aircraftCh)

	// Send initial snapshot
	aircraft := s.receiver.GetAircraft()
	if err := wsWriteJSON(ws, aircraft); err != nil {
		return nil
	}

	ticker := time.NewTicker(wsPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case aircraft, ok := <-aircraftCh:
			if !ok {
				return nil
			}
			if err := wsWriteJSON(ws, aircraft); err != nil {
				return nil
			}
		case <-ticker.C:
			if err := wsWriteMessage(ws, websocket.PingMessage, nil); err != nil {
				return nil
			}
		}
	}
}

// handleGetAircraft returns the current list of tracked aircraft.
func (s *Server) handleGetAircraft(c echo.Context) error {
	aircraft := s.receiver.GetAircraft()
	return c.JSON(http.StatusOK, map[string]any{"aircraft": aircraft})
}

// handleGetAircraftHistory returns all aircraft ever tracked (including
// those no longer active), sorted by LastSeen descending.
func (s *Server) handleGetAircraftHistory(c echo.Context) error {
	aircraft := s.receiver.GetAircraftHistory()
	return c.JSON(http.StatusOK, map[string]any{"aircraft": aircraft})
}

// handleGetAllAircraft returns both active and historical aircraft in a
// single list. Active aircraft come first.
func (s *Server) handleGetAllAircraft(c echo.Context) error {
	aircraft := s.receiver.GetAllAircraft()
	return c.JSON(http.StatusOK, map[string]any{"aircraft": aircraft})
}

type receiverPositionRequest struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (s *Server) handleSetReceiverPosition(c echo.Context) error {
	var req receiverPositionRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	s.receiver.SetReceiverPosition(req.Latitude, req.Longitude)
	return c.JSON(http.StatusOK, req)
}

// handleADSBStats returns ADS-B decoder statistics for debugging.
func (s *Server) handleADSBStats(c echo.Context) error {
	detected, valid, accepted, aircraftCount := s.receiver.GetADSBStats()
	return c.JSON(http.StatusOK, map[string]int{
		"detected": detected,
		"valid":    valid,
		"accepted": accepted,
		"aircraft": aircraftCount,
	})
}

// handleGetLRPTSatellites returns the list of Meteor-M satellites
// transmitting LRPT.
func (s *Server) handleGetLRPTSatellites(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{"satellites": lrpt.Satellites()})
}

// handleLRPTStats returns LRPT decoder statistics plus a constellation
// snapshot (recent soft symbols, interleaved I,Q).
func (s *Server) handleLRPTStats(c echo.Context) error {
	stats := s.receiver.GetLRPTStats()
	return c.JSON(http.StatusOK, map[string]any{
		"stats":         stats,
		"constellation": s.receiver.GetLRPTConstellation(),
	})
}

// handleResetLRPT clears the LRPT decoder state.
func (s *Server) handleResetLRPT(c echo.Context) error {
	s.receiver.ResetLRPT()
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// handleWSLRPT streams decoded LRPT image segments over WebSocket.
// Binary protocol (little-endian):
//
//	[u16 apid] [u32 strip] [u8 mcuIndex] [u8 reserved] [896 bytes pixels]
//
// Pixels are 8 rows × 112 columns (14 MCUs) of 8-bit grayscale; the
// segment's top-left corner is at (mcuIndex*8, strip*8).
func (s *Server) handleWSLRPT(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = ws.Close() }()
	startWSReadPump(ws)

	segCh := s.receiver.SubscribeLRPT()
	defer s.receiver.UnsubscribeLRPT(segCh)

	ticker := time.NewTicker(wsPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case seg, ok := <-segCh:
			if !ok {
				return nil
			}
			buf := make([]byte, 8+len(seg.Pixels))
			binary.LittleEndian.PutUint16(buf[0:2], uint16(seg.APID))
			binary.LittleEndian.PutUint32(buf[2:6], uint32(seg.Strip))
			buf[6] = byte(seg.MCUIndex)
			copy(buf[8:], seg.Pixels)
			if err := wsWriteMessage(ws, websocket.BinaryMessage, buf); err != nil {
				return nil
			}
		case <-ticker.C:
			if err := wsWriteMessage(ws, websocket.PingMessage, nil); err != nil {
				return nil
			}
		}
	}
}
