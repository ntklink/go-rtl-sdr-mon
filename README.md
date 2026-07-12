# GoEther-SDR

A Go-based SDR receiver inspired by [gqrx](https://github.com/gqrx-sdr/gqrx), with a web UI for remote operation and single-binary deployment. Supports broadcast FM/AM/SSB/CW, ADS-B aircraft tracking, and Meteor-M LRPT weather satellite imagery. Tested with the RTL-SDR Blog V4 & V3 dongles (RTL2832U + R828D, TCXO, bias-tee, HF).

![Overview](docs/image-overview.jpg)

![ADS-B](docs/image-ads-b.jpg)

## Features

- **RTL-SDR Support** — Built on [go-rtl-sdr](https://github.com/ntklink/go-rtl-sdr) CGO bindings; multi-device support via a pluggable `SDRDevice` interface. Tested with the [RTL-SDR Blog V4](https://www.rtl-sdr.com/buy-rtl-sdr-dvb-t-dongles/) dongle (RTL2832U + R828D tuner, TCXO, bias-tee, HF direct-sampling)
- **Web UI** — Vue 3 + [Reka UI](https://github.com/unovue/reka-ui), embedded into the Go binary via `//go:embed`
- **Real-time Spectrum & Waterfall** — Canvas-rendered, streamed over WebSocket with configurable FFT size, rate, averaging, and max-hold
- **Browser Audio Playback** — Demodulated PCM audio streamed over WebSocket, played with the Web Audio API
- **14 Demodulation Modes** — OFF, Raw I/Q, AM, AM-Sync, LSB, USB, CW-L, CW-U, NFM, WFM, WFM-Stereo, WFM-OIRT, ADS-B, LRPT (gqrx-compatible + satellite)
- **ADS-B Reception** — Decode Mode S Extended Squitter (1090 MHz) messages: Manchester decoding, CRC verification with single-bit error correction, CPR (Compact Position Reporting) position decoding, aircraft tracking with callsign/altitude/speed/heading/vertical-rate extraction
- **Meteor-M LRPT Weather Satellite** — Full digital decode of LRPT (Low Rate Picture Transmission) imagery from Meteor-M N2-3/N2-4 (137 MHz, the successor to NOAA APT which ended with the POES decommissioning): QPSK 72k demodulation (RRC matched filter, Costas loop, Gardner timing recovery, FFT-based carrier acquisition), soft-decision Viterbi (K=7, r=1/2), CCSDS deframing with derandomization, Reed-Solomon (255,223)×4 error correction, CCSDS space packet reassembly, MSU-MR JPEG decoding (per-channel APID 64-69), live constellation diagram, real-time segment-by-segment rendering, per-channel PNG export
- **Live Aircraft Map** — Leaflet-based map showing nearby aircraft positions with locale-aware tile layers (OpenStreetMap / AutoNavi); auto-requests browser geolocation for receiver position
- **Full DSP Chain** — DDC, FIR bandpass filtering, AGC with presets, anti-aliased audio resampling
- **gqrx-compatible Parameters** — AGC presets (Off/Slow/Medium/Fast), filter presets (Wide/Normal/Narrow), filter shapes (Soft/Normal/Sharp), CW offset, WFM de-emphasis
- **Internationalization** — English / Chinese UI with locale toggle
- **State Recovery** — All settings are synced from the backend on page refresh; audio playback survives tab switches

## Architecture

```
RTL-SDR → IQ Stream → ┌→ FFT (spectrum/waterfall) → WebSocket → Canvas
                      └→ DDC → Bandpass Filter → Demod → AGC → Resampler → WebSocket → Web Audio API
                      └→ ADS-B Decoder → Aircraft Tracker → WebSocket → Leaflet Map
                      └→ LRPT Decoder (QPSK → Viterbi → RS → JPEG) → WebSocket → Canvas Image
```

### DSP Signal Chain

| Stage              | File                                 | Description                                                                               |
| ------------------ | ------------------------------------ | ----------------------------------------------------------------------------------------- |
| Source             | `internal/sdr/source.go`             | RTL-SDR async read, 8-bit IQ → complex128                                                 |
| Device Abstraction | `internal/sdr/device.go`             | `SDRDevice` interface, `DeviceManager` for enumeration & hot-swap                         |
| FFT                | `internal/sdr/fft.go`                | Custom radix-2 Cooley-Tukey FFT, Hann window, max-hold with decay                         |
| DDC                | `internal/sdr/ddc.go`                | Digital down-converter (NCO + FIR low-pass + decimation)                                  |
| Filter             | `internal/sdr/filter.go`             | Windowed-sinc FIR design (low-pass / band-pass / complex band-pass)                       |
| Demodulator        | `internal/demod/`                    | FM, WFM (mono/stereo/OIRT), AM, AM-Sync (PLL), SSB                                        |
| AGC                | `internal/sdr/agc.go`                | AGC with hang, gqrx-matched presets                                                       |
| Resampler          | `internal/sdr/resampler.go`          | Anti-aliased FIR + linear interpolation to 48 kHz                                         |
| Receiver           | `internal/sdr/receiver.go`           | Top-level orchestration, per-client pub/sub for FFT & audio, source hot-swap              |
| ADS-B Decoder      | `internal/adsb/decoder.go`           | IQ → preamble detection → Manchester decoding → CRC verification                          |
| ADS-B Messages     | `internal/adsb/message.go`           | Callsign, altitude, airborne position, velocity extraction                                |
| ADS-B CPR          | `internal/adsb/cpr.go`               | Compact Position Reporting (global + relative) decoding                                   |
| ADS-B CRC          | `internal/adsb/crc.go`               | Mode S 24-bit CRC with single-bit error correction                                        |
| ADS-B Tracker      | `internal/adsb/tracker.go`           | Multi-aircraft tracking, ICAO-based state merging, CPR caching                            |
| LRPT Demodulator   | `internal/lrpt/demod.go`             | QPSK 72k: AGC, Costas loop, RRC matched filter, Gardner timing, FFT carrier acquisition   |
| LRPT FEC           | `internal/lrpt/viterbi.go` / `rs.go` | Soft-decision Viterbi (K=7, r=1/2, CCSDS 0171/0133); Reed-Solomon (255,223) ×4 interleave |
| LRPT Deframer      | `internal/lrpt/deframer.go`          | ASM correlation with 8-fold QPSK ambiguity resolution, PN derandomization                 |
| LRPT Packets       | `internal/lrpt/packet.go`            | VCDU → CCSDS space packet reassembly, MSU-MR segment extraction                           |
| LRPT JPEG          | `internal/lrpt/jpeg.go`              | MSU-MR JPEG: Annex K Huffman tables, quality-scaled dequantization, IDCT                  |

## Build

### Prerequisites

```bash
# RTL-SDR library and headers
sudo apt install librtlsdr-dev libusb-1.0-0-dev

# Go 1.22+ and Node.js 18+
```

### Makefile (recommended)

```bash
make build    # Build complete single binary (frontend + backend) → bin/
make run      # Build and run
make web      # Build frontend only
make dev      # Start Vite dev server (proxies API to backend)
make clean    # Clean build artifacts
```

### Manual Build

```bash
cd web && npm install && npm run build    # Frontend → web/dist/
go build -trimpath -ldflags="-s -w" -o bin/goether-sdr .
```

**Prerequisites:**

```bash
# Install QEMU binfmt handlers (one-time, for cross-architecture emulation)
docker run --privileged --rm tonistiigi/binfmt --install all

# Create a Docker Buildx builder that supports all platforms
docker buildx create --name mybuilder --use --driver docker-container
docker buildx inspect --bootstrap
```

**Build commands:**

```bash
make build-amd64    # → bin/goether-sdr-linux-amd64  (x86_64)
make build-arm64    # → bin/goether-sdr-linux-arm64  (aarch64)
make build-arm      # → bin/goether-sdr-linux-arm    (armv7)
make build-all      # All three architectures

make dist           # Build all + package into tarballs in dist/
```

## Usage

```bash
# Defaults: device 0, 1.8 MHz sample rate, 102.8 MHz, port 8080
./goether-sdr

# Custom parameters
./goether-sdr -device 0 -freq 144500000 -samplerate 2400000 -port 8080 -ppm 1

# Manual gain
./goether-sdr -autogain=false -gain 248  # 24.8 dB (gqrx default)
```

Open `https://localhost:8080` in your browser.

> The server uses HTTPS by default (required for browser geolocation). A self-signed certificate (`cert.pem` / `key.pem`) is auto-generated in the binary's directory on first run. Accept the browser's security warning to proceed. Use `-tls=false` to disable.

### CLI Flags

| Flag          | Default     | Description                                           |
| ------------- | ----------- | ----------------------------------------------------- |
| `-device`     | `0`         | RTL-SDR device index                                  |
| `-samplerate` | `1800000`   | Sample rate in Hz (gqrx default: 1.8 MHz)             |
| `-freq`       | `102800000` | Center frequency in Hz (default: 102.8 MHz)           |
| `-port`       | `8080`      | HTTP server port                                      |
| `-tls`        | `true`      | Use HTTPS (auto-generates self-signed cert if needed) |
| `-autogain`   | `true`      | Enable SDR auto gain                                  |
| `-gain`       | `248`       | Manual gain in 0.1 dB (248 = 24.8 dB, gqrx default)   |
| `-ppm`        | `0`         | Frequency correction in ppm                           |

## API Reference

### REST

| Method | Endpoint                 | Body                                  | Description                             |
| ------ | ------------------------ | ------------------------------------- | --------------------------------------- |
| GET    | `/api/device`            | —                                     | Active device info                      |
| GET    | `/api/devices`           | —                                     | List available devices                  |
| POST   | `/api/device/select`     | `{"id":"..."}`                        | Select / open a device                  |
| GET    | `/api/status`            | —                                     | Receiver status & config                |
| GET    | `/api/demods`            | —                                     | Available demodulator list              |
| POST   | `/api/frequency`         | `{"frequency":100000000}`             | Set center frequency                    |
| POST   | `/api/demod`             | `{"demod":"NFM"}`                     | Set demodulator                         |
| POST   | `/api/filter`            | `{"low":-5000,"high":5000}`           | Set filter cutoffs                      |
| POST   | `/api/filter-offset`     | `{"offset":0}`                        | Set filter offset                       |
| POST   | `/api/filter-shape`      | `{"shape":"normal"}`                  | Set filter shape (soft/normal/sharp)    |
| POST   | `/api/filter-preset`     | `{"preset":"normal"}`                 | Set filter preset (wide/normal/narrow)  |
| POST   | `/api/squelch`           | `{"level":-80}`                       | Set squelch level (dBFS)                |
| POST   | `/api/agc`               | `{"enabled":true}`                    | Enable/disable AGC                      |
| POST   | `/api/agc-preset`        | `{"preset":"medium"}`                 | Set AGC preset (off/slow/medium/fast)   |
| POST   | `/api/gain`              | `{"gain":248}`                        | Set manual gain (0.1 dB)                |
| POST   | `/api/auto-gain`         | `{"auto":true}`                       | Enable/disable SDR auto gain            |
| POST   | `/api/freq-correction`   | `{"ppm":1}`                           | Set frequency correction                |
| POST   | `/api/cw-offset`         | `{"offset":700}`                      | Set CW/BFO offset (Hz)                  |
| POST   | `/api/spectrum-avg`      | `{"avg":0.3}`                         | Set FFT averaging factor                |
| POST   | `/api/fft-size`          | `{"size":8192}`                       | Set FFT size                            |
| POST   | `/api/fft-rate`          | `{"rate":25}`                         | Set FFT refresh rate (fps)              |
| POST   | `/api/fft-max-hold`      | `{"enabled":true}`                    | Enable/disable max-hold                 |
| POST   | `/api/receiver-position` | `{"latitude":39.9,"longitude":116.4}` | Set receiver position (for ADS-B CPR)   |
| GET    | `/api/aircraft`          | —                                     | Current tracked aircraft list           |
| GET    | `/api/lrpt/satellites`   | —                                     | List of Meteor-M LRPT satellites        |
| GET    | `/api/lrpt-stats`        | —                                     | LRPT decoder statistics + constellation |
| POST   | `/api/lrpt-reset`        | `{}`                                  | Reset LRPT decoder state                |

### WebSocket

| Endpoint           | Format                                                                                   | Description                                          |
| ------------------ | ---------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| `/api/ws/fft`      | Binary: `4-byte size` → `float32[]` frames                                               | FFT spectrum data                                    |
| `/api/ws/audio`    | Binary: `1-byte channels` + `4-byte count` + `int16[]` samples                           | Audio PCM                                            |
| `/api/ws/status`   | JSON                                                                                     | Receiver status updates (500 ms interval)            |
| `/api/ws/aircraft` | JSON: `Aircraft[]`                                                                       | ADS-B aircraft positions (broadcast every 10 blocks) |
| `/api/ws/lrpt`     | Binary: `u16 apid` + `u32 strip` + `u8 mcuIndex` + `u8 rsvd` + `896-byte pixels` (112×8) | Meteor-M LRPT image segments                         |
