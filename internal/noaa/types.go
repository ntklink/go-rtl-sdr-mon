package noaa

// Satellite describes a NOAA polar-orbiting satellite that transmits APT.
type Satellite struct {
	Name      string  `json:"name"`
	Frequency uint32  `json:"frequency"` // Hz
	Period    float64 `json:"period"`    // orbital period in minutes
	Status    string  `json:"status"`
}

// Satellites returns the list of NOAA satellites that transmit APT imagery.
func Satellites() []Satellite {
	return []Satellite{
		{Name: "NOAA-15", Frequency: 137620000, Period: 101.4, Status: "active"},
		{Name: "NOAA-18", Frequency: 137912500, Period: 102.1, Status: "active"},
		{Name: "NOAA-19", Frequency: 137100000, Period: 102.5, Status: "active"},
	}
}

// APTLine represents one decoded line of an APT image.
// Pixels is 2080 bytes: channel A (sync+space+image+telemetry) then channel B.
type APTLine struct {
	LineNum int    `json:"lineNum"`
	Pixels  []byte `json:"pixels"` // 2080 bytes, 8-bit grayscale
}

// APTImage holds the accumulated APT image data.
type APTImage struct {
	Width      int `json:"width"`      // 2080
	Height     int `json:"height"`     // number of decoded lines
	LineNum    int `json:"lineNum"`    // starting line number of the image
	SyncFound  int `json:"syncFound"`  // number of sync frames detected
	LinesTotal int `json:"linesTotal"` // total lines processed
}

// APT pixel geometry constants (per line, 2080 pixels total at 4160 Hz).
const (
	LinePixels = 2080 // total pixels per line

	SyncAStart  = 0    // sync A start
	SyncAEnd    = 31   // sync A end (~28 pixels + margin)
	SpaceAStart = 31   // space A start
	SpaceAEnd   = 54   // space A end
	ImageAStart = 54   // channel A image start
	ImageAEnd   = 990  // channel A image end (~936 pixels)
	TeleAStart  = 990  // telemetry A start
	TeleAEnd    = 1040 // telemetry A end

	SyncBStart  = 1040 // sync B start
	SyncBEnd    = 1075 // sync B end (~35 pixels)
	SpaceBStart = 1075 // space B start
	SpaceBEnd   = 1098 // space B end
	ImageBStart = 1098 // channel B image start
	ImageBEnd   = 2034 // channel B image end
	TeleBStart  = 2034 // telemetry B start
	TeleBEnd    = 2080 // telemetry B end

	ImageAW = ImageAEnd - ImageAStart // ~936
	ImageBW = ImageBEnd - ImageBStart // ~936
)

// PixelRate is the APT pixel sample rate (2080 pixels / 0.5s = 4160 Hz).
const PixelRate = 4160.0

// LineDuration is the duration of one APT line in seconds.
const LineDuration = 0.5
