package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/ntklink/go-rtl-sdr-mon/sdr"
)

// --- Utility functions (migrated from console.go) ---

// formatFreq formats a frequency in Hz to a human-readable string.
func formatFreq(hz uint32) string {
	if hz >= 1_000_000_000 {
		return fmt.Sprintf("%.3f GHz", float64(hz)/1e9)
	}
	if hz >= 1_000_000 {
		return fmt.Sprintf("%.3f MHz", float64(hz)/1e6)
	}
	if hz >= 1_000 {
		return fmt.Sprintf("%.3f kHz", float64(hz)/1e3)
	}
	return fmt.Sprintf("%d Hz", hz)
}

// formatGain formats a gain value (in tenths of dB) to a string.
func formatGain(gain int) string {
	return fmt.Sprintf("%.1f dB", float64(gain)/10.0)
}

// demodOptions returns the list of available demodulator names (matches gqrx order).
func demodOptions() []string {
	return []string{
		"OFF", "Raw I/Q", "AM", "AM-Sync", "LSB", "USB",
		"CW-L", "CW-U", "NFM", "WFM", "WFM-Stereo", "WFM-OIRT",
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
	source   *sdr.Source
}

// NewServer creates a new Server.
func NewServer(source *sdr.Source, receiver *sdr.Receiver) *Server {
	return &Server{
		receiver: receiver,
		source:   source,
	}
}

// RegisterRoutes registers all API routes on the Echo instance.
func (s *Server) RegisterRoutes(e *echo.Echo) {
	api := e.Group("/api")

	// Device info
	api.GET("/device", s.handleDeviceInfo)

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
}

// --- REST API Handlers ---

func (s *Server) handleDeviceInfo(c echo.Context) error {
	info, err := s.source.Info()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, info)
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

func (s *Server) handleSetFFTSize(c echo.Context) error {
	var req fftSizeRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
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

// handleWSFFT streams FFT spectrum data over WebSocket.
func (s *Server) handleWSFFT(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	fftCh := s.receiver.FFTCh()

	// Send FFT size first
	size := s.receiver.GetSpectrumSize()
	sizeBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(sizeBuf, uint32(size))
	if err := ws.WriteMessage(websocket.BinaryMessage, sizeBuf); err != nil {
		return nil
	}

	for {
		select {
		case data, ok := <-fftCh:
			if !ok {
				return nil
			}
			// Convert float32 slice to bytes
			buf := make([]byte, len(data)*4)
			for i, v := range data {
				binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
			}
			if err := ws.WriteMessage(websocket.BinaryMessage, buf); err != nil {
				return nil
			}
		case <-time.After(30 * time.Second):
			// Keepalive
			if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return nil
			}
		}
	}
}

// handleWSAudio streams audio PCM data over WebSocket.
func (s *Server) handleWSAudio(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	audioCh := s.receiver.AudioCh()
	var writeMu sync.Mutex

	// Reader for close detection
	go func() {
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		audio, ok := <-audioCh
		if !ok {
			return nil
		}

		// Format: [1 byte: channels] [4 bytes: sample count] [samples...]
		// Each sample is int16 (2 bytes)
		// Mono: left only, Stereo: interleaved L,R,L,R...
		channels := 1
		if audio.Right != nil {
			channels = 2
		}

		numSamples := len(audio.Left)
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
		err := ws.WriteMessage(websocket.BinaryMessage, buf)
		writeMu.Unlock()
		if err != nil {
			return nil
		}
	}
}

// handleWSStatus streams status updates over WebSocket.
func (s *Server) handleWSStatus(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	statusCh := s.receiver.StatusCh()

	// Also send periodic updates
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case status, ok := <-statusCh:
			if !ok {
				return nil
			}
			if err := ws.WriteJSON(status); err != nil {
				return nil
			}
		case <-ticker.C:
			status := s.receiver.GetStatus()
			if err := ws.WriteJSON(status); err != nil {
				return nil
			}
		}
	}
}
