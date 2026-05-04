package modes

import "math"

// Airborne Velocity (TC 19, subtype 1)
// ====================================
//
// ICAO Annex 10 Vol IV §3.1.2.9.5 / DO-260B §2.2.3.2.6. The ADS-B
// velocity message has four sub-formats keyed off a 3-bit Subtype
// field; this commit decodes the most common one — Subtype 1,
// Ground Speed, normal subsonic range — and surfaces the
// supersonic / airspeed paths as errVelocitySubtypeUnsupported
// for follow-up commits to fill in once a real-frame test vector
// is in hand.
//
// Wire layout for Subtype 1 (ME bit positions 0..55, MSB-first):
//
//	bit  0..4   TC (5 = 19)
//	bit  5..7   Subtype (3 bits, value 1 here)
//	bit  8      Intent Change Flag (IC)
//	bit  9      IFR Capability Flag
//	bit 10..12  Navigation Uncertainty Category (NUCv)
//	bit 13      East-West direction (0 = east, 1 = west)
//	bit 14..23  East-West velocity, raw v-1 in knots (0 = no value)
//	bit 24      North-South direction (0 = north, 1 = south)
//	bit 25..34  North-South velocity, raw v-1 in knots
//	bit 35      Vertical-rate source (0 = GNSS, 1 = baro)
//	bit 36      Vertical-rate sign (0 = up, 1 = down)
//	bit 37..45  Vertical-rate magnitude, raw v-1 × 64 ft/min
//	bit 46..47  Reserved
//	bit 48      GNSS-baro altitude difference sign
//	bit 49..55  GNSS-minus-baro altitude, raw v-1 × 25 ft

// VelocitySubtype enumerates the four ME-velocity wire layouts.
type VelocitySubtype uint8

// Documented subtype values per DO-260B table 2-9.
const (
	VelocitySubtypeGroundSpeedSubsonic   VelocitySubtype = 1
	VelocitySubtypeGroundSpeedSupersonic VelocitySubtype = 2
	VelocitySubtypeAirspeedSubsonic      VelocitySubtype = 3
	VelocitySubtypeAirspeedSupersonic    VelocitySubtype = 4
)

// VerticalRateSource indicates whether the vertical rate is
// derived from GNSS or barometric instruments.
type VerticalRateSource uint8

// Documented VR-source values.
const (
	VerticalRateSourceGNSS VerticalRateSource = 0
	VerticalRateSourceBaro VerticalRateSource = 1
)

// AirborneVelocityMessage is the decoded payload of an ADS-B
// velocity message (TC 19). Only Subtype 1 (ground-speed
// subsonic) is fully populated today; the airspeed subtypes 3/4
// surface the Subtype field with the rest of the velocity values
// at zero (and Available flags false) so consumers can detect
// the "we know it's velocity but the variant isn't decoded yet"
// state without crashing.
type AirborneVelocityMessage struct {
	Subtype               VelocitySubtype
	IntentChange          bool
	IFRCapability         bool
	NavigationUncertainty uint8

	GroundSpeedKnots     float64
	TrackDegrees         float64
	GroundSpeedAvailable bool

	VerticalRateSource    VerticalRateSource
	VerticalRateFeetMin   int
	VerticalRateAvailable bool

	GNSSMinusBaroAltFt     int
	GNSSMinusBaroAvailable bool
}

func (AirborneVelocityMessage) isModesMessage() {}

// decodeVelocity parses a TC 19 ME payload. mePayload must be 7
// bytes (the ES dispatcher enforces that). For Subtype 1 the
// returned message has full ground-speed + track + vertical-rate
// values; for the other subtypes only the structural fields
// (Subtype, IC, IFR, NUCv) are populated until the per-subtype
// follow-up lands.
func decodeVelocity(mePayload []byte) AirborneVelocityMessage {
	const (
		subtypeMask byte = 0x07
		nucvShift   byte = 3
		nucvMask    byte = 0x07

		intentBit byte = 0x80
		ifrBit    byte = 0x40
	)

	subtype := VelocitySubtype(mePayload[0] & subtypeMask)

	out := AirborneVelocityMessage{
		Subtype:               subtype,
		IntentChange:          mePayload[1]&intentBit != 0,
		IFRCapability:         mePayload[1]&ifrBit != 0,
		NavigationUncertainty: (mePayload[1] >> nucvShift) & nucvMask,
	}

	multiplier := 1
	if subtype == VelocitySubtypeGroundSpeedSupersonic {
		multiplier = 4 //nolint:mnd // supersonic ×4 per §3.1.2.9.5.2.
	}

	if subtype == VelocitySubtypeGroundSpeedSubsonic ||
		subtype == VelocitySubtypeGroundSpeedSupersonic {
		decodeGroundSpeedFields(&out, mePayload, multiplier)
		decodeVerticalRate(&out, mePayload)
		decodeGNSSBaroDelta(&out, mePayload)
	}

	return out
}

// decodeGroundSpeedFields fills the GroundSpeed / Track fields
// for subtypes 1 and 2.
func decodeGroundSpeedFields(out *AirborneVelocityMessage, mePayload []byte, multiplier int) {
	const (
		ewDirMask  byte = 0x04
		nsDirMask  byte = 0x80
		velMask    uint = 0x3FF
		ewHighMask uint = 0x03
		nsHighMask uint = 0x7F
		nsLowShift byte = 5
		degPerRad       = 180.0 / math.Pi
		fullCircle      = 360.0
	)

	ewWest := mePayload[1]&ewDirMask != 0
	nsSouth := mePayload[3]&nsDirMask != 0

	ewRaw := (uint(mePayload[1])&ewHighMask)<<8 | uint(mePayload[2]) //nolint:mnd // 8-bit byte assembly.
	nsRaw := (uint(mePayload[3])&nsHighMask)<<3 | uint(mePayload[4])>>nsLowShift

	ewRaw &= velMask
	nsRaw &= velMask

	if ewRaw == 0 && nsRaw == 0 {
		return
	}

	ewKnots := float64(int(ewRaw)-1) * float64(multiplier)
	if ewWest {
		ewKnots = -ewKnots
	}

	nsKnots := float64(int(nsRaw)-1) * float64(multiplier)
	if nsSouth {
		nsKnots = -nsKnots
	}

	out.GroundSpeedKnots = math.Hypot(ewKnots, nsKnots)
	out.TrackDegrees = math.Atan2(ewKnots, nsKnots) * degPerRad

	if out.TrackDegrees < 0 {
		out.TrackDegrees += fullCircle
	}

	out.GroundSpeedAvailable = true
}

// decodeVerticalRate fills the VR fields. ME bits 35..45 span
// the lower 4 bits of byte 4 plus byte 5's top 6 bits.
func decodeVerticalRate(out *AirborneVelocityMessage, mePayload []byte) {
	const (
		vrSrcMask  byte = 0x10
		vrSignMask byte = 0x08
		vrHighMask uint = 0x07
		vrLowShift byte = 2
		ftPerStep       = 64
	)

	vrSourceBaro := mePayload[4]&vrSrcMask != 0
	vrSignDown := mePayload[4]&vrSignMask != 0
	vrRaw := (uint(mePayload[4])&vrHighMask)<<6 | uint(mePayload[5])>>vrLowShift //nolint:mnd // 6-bit upper carry.

	if vrRaw == 0 {
		return
	}

	out.VerticalRateAvailable = true
	if vrSourceBaro {
		out.VerticalRateSource = VerticalRateSourceBaro
	}

	rate := (int(vrRaw) - 1) * ftPerStep
	if vrSignDown {
		rate = -rate
	}

	out.VerticalRateFeetMin = rate
}

// decodeGNSSBaroDelta fills the GNSS-minus-baro altitude delta
// (ME bits 48..55).
func decodeGNSSBaroDelta(out *AirborneVelocityMessage, mePayload []byte) {
	const (
		signMask byte = 0x80
		valMask  byte = 0x7F
		ftPerLSB      = 25
	)

	raw := mePayload[6] & valMask
	if raw == 0 {
		return
	}

	out.GNSSMinusBaroAvailable = true

	delta := (int(raw) - 1) * ftPerLSB
	if mePayload[6]&signMask != 0 {
		delta = -delta
	}

	out.GNSSMinusBaroAltFt = delta
}
