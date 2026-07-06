package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/ntklink/go-rtl-sdr-mon/internal/sdr"
)

//go:embed web/dist/*
var content embed.FS

func main() {
	// CLI flags
	deviceIndex := flag.Int("device", 0, "RTL-SDR device index")
	sampleRate := flag.Uint("samplerate", 1800000, "Sample rate in Hz (gqrx default: 1.8 MHz)")
	frequency := flag.Uint("freq", 102800000, "Center frequency in Hz (default: 102.8 MHz)")
	port := flag.Int("port", 8080, "HTTP server port")
	autoGain := flag.Bool("autogain", true, "Enable auto gain")
	gain := flag.Int("gain", 248, "Manual gain in tenths of dB (248 = 24.8 dB, gqrx default)")
	ppm := flag.Int("ppm", 0, "Frequency correction in ppm")
	useTLS := flag.Bool("tls", true, "Use HTTPS (auto-generates self-signed cert if needed)")
	flag.Parse()

	// Create device manager with RTL-SDR enumerator
	dm := sdr.NewDeviceManager(sdr.RTLSDREnumerator{})

	// Enumerate available devices
	devices := dm.Enumerate()
	if len(devices) == 0 {
		log.Fatalf("No SDR devices found")
	}
	log.Printf("Found %d SDR device(s):", len(devices))
	for _, d := range devices {
		log.Printf("  [%s] %s — %s (serial: %s)", d.ID, d.Name, d.Product, d.Serial)
	}

	// Open the specified device (default: first device)
	openID := devices[0].ID
	if *deviceIndex >= 0 && *deviceIndex < len(devices) {
		openID = devices[*deviceIndex].ID
	}
	log.Printf("Opening device %s (sample rate: %d, freq: %d)...", openID, *sampleRate, *frequency)
	source, err := dm.Open(openID, uint32(*sampleRate), uint32(*frequency))
	if err != nil {
		log.Fatalf("Failed to open SDR: %v", err)
	}
	defer dm.CloseAll()

	// Print device info
	info, _ := source.Info()
	log.Printf("Device: %s (tuner: %s, serial: %s)", info.Product, info.TunerType, info.Serial)

	// Set frequency correction
	if *ppm != 0 {
		if err := source.SetFreqCorrection(*ppm); err != nil {
			log.Printf("Warning: set freq correction: %v", err)
		}
	}

	// Set gain
	if !*autoGain {
		if err := source.SetAutoGain(false); err != nil {
			log.Printf("Warning: set auto gain: %v", err)
		}
		if *gain != 0 {
			if err := source.SetGain(*gain); err != nil {
				log.Printf("Warning: set gain: %v", err)
			}
		}
	}

	// Create receiver config
	config := sdr.DefaultReceiverConfig()
	config.SampleRate = uint32(*sampleRate)
	config.CenterFreq = uint32(*frequency)
	config.AutoGain = *autoGain
	config.Gain = *gain
	config.FreqCorrection = *ppm

	// Create receiver
	receiver := sdr.NewReceiver(source, config)

	// Start source (blocking, so in goroutine)
	go func() {
		log.Printf("Starting SDR source...")
		if err := source.Start(); err != nil {
			log.Printf("SDR source error: %v", err)
		}
	}()

	// Start receiver
	receiver.Start()
	log.Printf("Receiver started")

	// Create Echo server
	e := echo.New()
	e.HideBanner = true

	// CORS for development
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
	}))

	// Register API routes
	server := NewServer(dm, receiver)
	server.RegisterRoutes(e)

	// Serve Vue frontend (embedded)
	e.Use(middleware.StaticWithConfig(middleware.StaticConfig{
		HTML5:      true,
		Root:       "web/dist",
		Filesystem: http.FS(content),
	}))

	// Handle signals for clean shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Printf("Shutting down...")
		receiver.Stop()
		source.Stop()
		e.Close()
	}()

	// Start HTTP/HTTPS server
	addr := fmt.Sprintf(":%d", *port)
	if *useTLS {
		certFile, keyFile, err := EnsureTLSCert()
		if err != nil {
			log.Fatalf("Failed to generate TLS certificate: %v", err)
		}
		log.Printf("Web server starting on https://0.0.0.0%s", addr)
		if err := e.StartTLS(addr, certFile, keyFile); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	} else {
		log.Printf("Web server starting on http://0.0.0.0%s", addr)
		if err := e.Start(addr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}
}
