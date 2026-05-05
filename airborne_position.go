package modes

import "errors"

// Airborne Position (TC 9..18 barometric, TC 20..22 GNSS)
// =======================================================
//
// ICAO Annex 10 Vol IV §3.1.2.9.4 / DO-260B §A.1.4.3. ADS-B
// position messages broadcast at ~2 Hz, alternating between the
// even and odd CPR formats. TC values map to the altitude
// encoding and Navigation Integrity Category (NIC):
//
//	TC  9 .. 18 — barometric altitude, NIC tier per type code
//	TC 20 .. 22 — GNSS / geometric altitude
//
// Wire layout (ME bits 0..55, MSB-first; bytes 0..6):
//
//	bit  0..4   TC (5 bits)              [byte 0 high 5 bits]
//	bit  5..6   Surveillance Status (SS) [byte 0 bits 2..1]
//	bit  7      Single Antenna Flag      [byte 0 bit 0]
//	bit  8..19  Altitude (12 bits)       [byte 1 + byte 2 high 4 bits]
//	bit 20      Time bit (T)             [byte 2 bit 3]
//	bit 21      CPR Format (F)           [byte 2 bit 2]
//	bit 22..38  Latitude CPR (17 bits)   [byte 2 low 2 + byte 3 + byte 4 high 7]
//	bit 39..55  Longitude CPR (17 bits)  [byte 4 low 1 + byte 5 + byte 6]
//
// The 12-bit barometric altitude field is similar to (but
// distinct from) the 13-bit AC field of DF 0/4/16/20: here the
// M bit is omitted (always feet) and only the Q bit is present.
// With Q = 1 the remaining 11 bits form an unsigned N and
// altitude = 25·N − 1000 ft.

// SurveillanceStatus encodes the SS field on airborne and
// surface position messages per §3.1.2.9.4.2.
type SurveillanceStatus uint8

// Documented surveillance-status values.
const (
	SurveillanceStatusNoCondition    SurveillanceStatus = 0
	SurveillanceStatusPermanentAlert SurveillanceStatus = 1
	SurveillanceStatusTemporaryAlert SurveillanceStatus = 2
	SurveillanceStatusSPI            SurveillanceStatus = 3
)

// AirbornePositionMessage is the decoded payload of an ADS-B
// airborne position message (TC 9..18 or TC 20..22). Position
// resolution requires either a paired even/odd frame within the
// 10-second CPR window (DecodeCPRGlobal) or a known reference
// (DecodeCPRLocal); the message itself carries the raw 17-bit
// CPR values plus altitude + status flags.
type AirbornePositionMessage struct {
	TypeCode           TypeCode
	SurveillanceStatus SurveillanceStatus
	SingleAntennaFlag  bool
	AltitudeFeet       int
	AltitudeError      error
	TimeFlag           bool
	CPR                CPRPosition

	// IsGNSSAltitude reports whether the altitude encoding came
	// from TC 20..22 (geometric / GNSS-derived) rather than TC
	// 9..18 (barometric). Useful for consumers that want to
	// merge both sources sensibly.
	IsGNSSAltitude bool
}

func (AirbornePositionMessage) isModesMessage() {}

// errGNSSAltitudeUnsupported marks TC 20..22 GNSS-altitude
// encodings the decoder doesn't yet split per subtype.
var errGNSSAltitudeUnsupported = errors.New("modes: GNSS-altitude encoding (TC 20..22) not yet supported")

// decodeAirbornePosition parses a TC 9..18 / 20..22 ME payload.
// mePayload must be exactly 7 bytes.
func decodeAirbornePosition(typeCode TypeCode, mePayload []byte) AirbornePositionMessage {
	const (
		ssShift   byte     = 1
		ssMask    byte     = 0x03
		safMask   byte     = 0x01
		tBitMask  byte     = 0x08
		fBitMask  byte     = 0x04
		latHigh2  uint     = 0x03
		lonHigh1  uint     = 0x01
		gnssTCMin TypeCode = 20
		gnssTCMax TypeCode = 22
	)

	cprLat := (uint32(mePayload[2])&uint32(latHigh2))<<15 |
		uint32(mePayload[3])<<7 | //nolint:mnd // 8-bit byte at bits 24..31, 7 bits of next byte at 32..38.
		uint32(mePayload[4])>>1

	cprLon := (uint32(mePayload[4])&uint32(lonHigh1))<<16 |
		uint32(mePayload[5])<<8 |
		uint32(mePayload[6])

	out := AirbornePositionMessage{
		TypeCode:           typeCode,
		SurveillanceStatus: SurveillanceStatus((mePayload[0] >> ssShift) & ssMask),
		SingleAntennaFlag:  mePayload[0]&safMask != 0,
		IsGNSSAltitude:     typeCode >= gnssTCMin && typeCode <= gnssTCMax,
		TimeFlag:           mePayload[2]&tBitMask != 0,
		CPR: CPRPosition{
			Format:    cprFormatFromBit(mePayload[2]&fBitMask != 0),
			Latitude:  cprLat,
			Longitude: cprLon,
		},
	}

	altitude, err := decodeAirbornePositionAltitude(mePayload, out.IsGNSSAltitude)
	out.AltitudeFeet = altitude
	out.AltitudeError = err

	return out
}

// decodeAirbornePositionAltitude extracts the 12-bit altitude
// field from byte 1 + the top 4 bits of byte 2. For barometric
// encodings (Q=1) altitude is 25·N − 1000 ft.
//
//nolint:revive // isGNSS selects between two altitude encodings, not control flow.
func decodeAirbornePositionAltitude(mePayload []byte, isGNSS bool) (int, error) {
	const (
		// 12-bit altitude code: byte 1 (full) shifted left by 4
		// to make room for byte 2's top 4 bits.
		altByte1Shift uint   = 4
		altByte2Mask  byte   = 0xF0
		altMask       uint16 = 0x0FFF

		qBitMask      uint16 = 1 << 4
		altMultiplier        = 25
		altOffset            = -1000

		topMask  uint16 = 0xFE0
		topShift uint   = 1
		botMask  uint16 = 0x00F
	)

	if isGNSS {
		return 0, errGNSSAltitudeUnsupported
	}

	altCode := (uint16(mePayload[1])<<altByte1Shift |
		uint16(mePayload[2]&altByte2Mask)>>altByte1Shift) & altMask

	if altCode&qBitMask == 0 {
		return 0, errGillhamUnsupported
	}

	value := (altCode&topMask)>>topShift | (altCode & botMask)

	return int(value)*altMultiplier + altOffset, nil
}

//nolint:revive // formatBit names the wire-format flag, not a control toggle.
func cprFormatFromBit(formatBit bool) CPRFormat {
	if formatBit {
		return CPRFormatOdd
	}

	return CPRFormatEven
}
