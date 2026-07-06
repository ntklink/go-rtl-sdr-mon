package adsb

// Aircraft represents a tracked aircraft.
type Aircraft struct {
	ICAO         string  `json:"icao"`         // 6-digit hex ICAO address
	Callsign     string  `json:"callsign"`     // Flight callsign (e.g. "CSA123")
	Latitude     float64 `json:"latitude"`     // Decimal degrees
	Longitude    float64 `json:"longitude"`    // Decimal degrees
	Altitude     int     `json:"altitude"`     // Feet
	Speed        float64 `json:"speed"`        // Knots (ground speed)
	Track        float64 `json:"track"`        // Degrees (true heading)
	VerticalRate int     `json:"verticalRate"` // Feet/min (positive = climbing)
	Squawk       string  `json:"squawk"`       // 4-digit squawk code (octal)
	OnGround     bool    `json:"onGround"`
	TypeCode     int     `json:"typeCode"` // Last message type code
	LastSeen     int64   `json:"lastSeen"` // Unix timestamp (ms)
	MessageCount int     `json:"messageCount"`
}

// Message represents a decoded ADS-B message (DF=17/18 extended squitter).
type Message struct {
	DF       int    // Downlink format (17 = ADS-B, 18 = TIS-B)
	CA       int    // Capability
	ICAO     string // 6-digit hex address
	TypeCode int    // Message type code (TC)
	ME       []byte // 7-byte message (56 bits)
	Parity   []byte // 3-byte parity
	Raw      []byte // 14-byte raw message (112 bits)
}

// Message type codes
const (
	TCIdentification   = 1  // Aircraft identification
	TCSurfacePosition  = 2  // Surface position (2-4)
	TCAirbornePosition = 9  // Airborne position (9-18)
	TCAirborneVelocity = 19 // Airborne velocities (19-22)
)
