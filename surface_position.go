package modes

// Surface Position (TC 5..8)
// ==========================
//
// ICAO Annex 10 Vol IV §3.1.2.9.4 / DO-260B §A.1.4.4. ADS-B
// surface-position messages are broadcast by aircraft (and
// taxiing surface vehicles like fuel trucks fitted with Mode S
// transponders) when on the ground. The wire layout differs
// from airborne position only in the first 12 bits of the ME
// payload — altitude is replaced by ground-speed (Movement) and
// heading.
//
// ME field layout (56 bits, MSB-first within ME):
//
//	bit  0..4   TC (5 bits, range 5..8)
//	bit  5..11  Movement (7 bits, ground speed, encoded scale)
//	bit 12      Heading Status (1 bit, 0 = no heading)
//	bit 13..19  Heading (7 bits, 360°/128 per LSB)
//	bit 20      Time bit (T)
//	bit 21      CPR Format (F)
//	bit 22..38  Latitude CPR (17 bits)
//	bit 39..55  Longitude CPR (17 bits)
//
// Movement field decoding (§3.1.2.9.4.4):
//
//	value | speed
//	------|--------------------------
//	0     | no info
//	1     | stopped (≤ 0.125 kt)
//	2..8  | 0.125 kt steps     →   0.125 .. 1.0 kt
//	9..12 | 0.25 kt steps      →   1.0   .. 2.0 kt
//	13..38| 0.5 kt steps       →   2.0   .. 15.0 kt
//	39..93| 1 kt steps         →  15.0   .. 70.0 kt
//	94..108| 2 kt steps        →  70.0   .. 100.0 kt
//	109..123| 5 kt steps       → 100.0   .. 175.0 kt
//	124   | > 175 kt
//	125..127 | reserved

// SurfacePositionMessage is the decoded payload of a TC 5..8
// surface-position ME field. Position resolution requires CPR
// pairing (DecodeCPRGlobal) or a reference (DecodeCPRLocal) —
// same model as airborne position.
type SurfacePositionMessage struct {
	TypeCode TypeCode

	// GroundSpeedKnots is decoded from the 7-bit Movement field
	// per the variable-step table at §3.1.2.9.4.4. Zero with
	// GroundSpeedAvailable false means no value.
	GroundSpeedKnots     float64
	GroundSpeedAvailable bool

	// HeadingDegrees is the magnetic heading in degrees [0, 360).
	// HeadingAvailable false when the Heading Status bit is zero.
	HeadingDegrees   float64
	HeadingAvailable bool

	TimeFlag bool
	CPR      CPRPosition
}

func (SurfacePositionMessage) isModesMessage() {}

// decodeSurfacePosition parses a TC 5..8 ME payload.
// mePayload must be exactly 7 bytes.
func decodeSurfacePosition(typeCode TypeCode, mePayload []byte) SurfacePositionMessage {
	const (
		headingStatusBit byte = 0x40
		tBitMask         byte = 0x08
		fBitMask         byte = 0x04
		latHigh2         uint = 0x03
		lonHigh1         uint = 0x01

		headingScale = 360.0 / 128.0
	)

	movementRaw := (uint(mePayload[0])&0x07)<<4 | uint(mePayload[1])>>4 //nolint:mnd // 3+4 bit assembly.
	headingStatus := mePayload[1]&headingStatusBit != 0
	headingRaw := (uint(mePayload[1])&0x07)<<4 | uint(mePayload[2])>>4 //nolint:mnd // 3+4 bit assembly.

	cprLat := (uint32(mePayload[2])&uint32(latHigh2))<<15 |
		uint32(mePayload[3])<<7 |
		uint32(mePayload[4])>>1

	cprLon := (uint32(mePayload[4])&uint32(lonHigh1))<<16 |
		uint32(mePayload[5])<<8 |
		uint32(mePayload[6])

	out := SurfacePositionMessage{
		TypeCode: typeCode,
		TimeFlag: mePayload[2]&tBitMask != 0,
		CPR: CPRPosition{
			Format:    cprFormatFromBit(mePayload[2]&fBitMask != 0),
			Latitude:  cprLat,
			Longitude: cprLon,
		},
	}

	if speed, ok := decodeMovementField(movementRaw); ok {
		out.GroundSpeedKnots = speed
		out.GroundSpeedAvailable = true
	}

	if headingStatus {
		out.HeadingDegrees = float64(headingRaw) * headingScale
		out.HeadingAvailable = true
	}

	return out
}

// decodeMovementField applies the spec's piecewise step table
// to a 7-bit Movement raw value, returning the speed in knots
// and a bool flag.
//
//nolint:mnd // movement field step table — values from the spec are the documentation.
func decodeMovementField(raw uint) (float64, bool) {
	switch {
	case raw == 0:
		return 0, false
	case raw == 1:
		return 0, true // "stopped" — knots = 0 with available flag true.
	case raw <= 8:
		return 0.125 + 0.125*float64(raw-2), true
	case raw <= 12:
		return 1.0 + 0.25*float64(raw-9), true
	case raw <= 38:
		return 2.0 + 0.5*float64(raw-13), true
	case raw <= 93:
		return 15.0 + 1.0*float64(raw-39), true
	case raw <= 108:
		return 70.0 + 2.0*float64(raw-94), true
	case raw <= 123:
		return 100.0 + 5.0*float64(raw-109), true
	case raw == 124:
		return 175.0, true // > 175 kt; report the lower bound and flag available.
	}

	return 0, false // 125..127 reserved.
}
