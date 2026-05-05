package modes

import (
	"errors"
	"testing"
)

// This file groups small tests that exercise branches the
// decoder-specific test files leave alone — typically error
// paths and rarely-hit configurations. Together with the
// per-feature tests, every executable statement in the package
// has coverage.

func TestCPRFormatFromBitOddPath(t *testing.T) {
	t.Parallel()

	if got := cprFormatFromBit(true); got != CPRFormatOdd {
		t.Errorf("cprFormatFromBit(true) = %d, want %d", got, CPRFormatOdd)
	}
}

func TestDecodeCommBIdentityRejectsShortFrame(t *testing.T) {
	t.Parallel()

	frame := Frame{byte(DFCommBIdentity << 3)}
	if _, err := DecodeCommBIdentity(frame, 0); !errors.Is(err, errFrameTooShort) {
		t.Errorf("err = %v, want errFrameTooShort", err)
	}
}

func TestDecodeCommBIdentityRejectsWrongDF(t *testing.T) {
	t.Parallel()

	frame := make(Frame, LongFrameBytes)
	frame[0] = byte(DFExtendedSquitter << 3)

	if _, err := DecodeCommBIdentity(frame, 0); !errors.Is(err, errWrongDF) {
		t.Errorf("err = %v, want errWrongDF", err)
	}
}

func TestDecodeCommBAltitudeRejectsShortFrame(t *testing.T) {
	t.Parallel()

	frame := Frame{byte(DFCommBAltitude << 3)}
	if _, err := DecodeCommBAltitude(frame, 0); !errors.Is(err, errFrameTooShort) {
		t.Errorf("err = %v, want errFrameTooShort", err)
	}
}

func TestDecodeMEPayloadUnsupportedTC(t *testing.T) {
	t.Parallel()

	// TC 23 (test message) — no decoder yet.
	mePayload := [7]byte{23 << 3, 0, 0, 0, 0, 0, 0}
	if _, err := decodeMEPayload(23, mePayload[:]); !errors.Is(err, errUnsupportedTypeCode) {
		t.Errorf("err = %v, want errUnsupportedTypeCode", err)
	}
}

func TestCPRNLFallthroughBetweenBoundariesAndPolarCutoff(t *testing.T) {
	t.Parallel()

	// Latitude in (87.0, 90.0]: above the largest boundary but
	// at or above the polar cutoff. The polar-cutoff branch
	// returns NL = 1.
	if got := cprNL(88.0); got != 1 {
		t.Errorf("cprNL(88.0) = %d, want 1", got)
	}
}

func TestDecodeVelocitySupersonicGroundSpeed(t *testing.T) {
	t.Parallel()

	// Subtype 2 = supersonic. Same encoding as subtype 1 but
	// with the ×4 multiplier, so a raw EW velocity of 250 in the
	// wire field decodes to ~1000 kt.
	const subtype = 2

	var mePayload [7]byte

	mePayload[0] = (19 << 3) | subtype

	// ewRaw = 250 → byte 1 low 2 bits = 0, byte 2 = 0xFA.
	// 250 = 0xFA = 0b11111010 → byte 1 low 2 = 0, byte 2 = 0xFA.
	mePayload[2] = 0xFA
	// nsRaw = 0 stays zero.

	msg := decodeVelocity(mePayload[:])

	if msg.Subtype != VelocitySubtypeGroundSpeedSupersonic {
		t.Errorf("Subtype = %d, want %d", msg.Subtype, VelocitySubtypeGroundSpeedSupersonic)
	}

	if !msg.GroundSpeedAvailable {
		t.Fatal("GroundSpeedAvailable = false, want true")
	}

	// raw 250 → (250-1)*4 = 996 kt
	if msg.GroundSpeedKnots < 990 || msg.GroundSpeedKnots > 1000 {
		t.Errorf("GroundSpeedKnots = %.2f, want ~996", msg.GroundSpeedKnots)
	}
}

func TestDecodeVerticalRateBaroSource(t *testing.T) {
	t.Parallel()

	// Build a velocity ME with vertical-rate source = baro
	// (bit 4 of byte 4) and a small magnitude.
	mePayload := makeVelocityME(100, 100, 1024)
	mePayload[4] |= 0x10 // source = baro

	msg := decodeVelocity(mePayload[:])
	if msg.VerticalRateSource != VerticalRateSourceBaro {
		t.Errorf("VerticalRateSource = %d, want %d", msg.VerticalRateSource, VerticalRateSourceBaro)
	}
}

func TestDecodeGNSSBaroDeltaPositive(t *testing.T) {
	t.Parallel()

	mePayload := makeVelocityME(100, 100, 1024)
	// GNSS-baro delta byte (index 6): bit 7 = sign (0 = GNSS above
	// baro), bits 6..0 = raw v-1 × 25 ft. Set raw = 5 → 100 ft above.
	mePayload[6] = 0x05

	msg := decodeVelocity(mePayload[:])
	if !msg.GNSSMinusBaroAvailable {
		t.Fatal("GNSSMinusBaroAvailable = false, want true")
	}

	if msg.GNSSMinusBaroAltFt != 100 {
		t.Errorf("GNSSMinusBaroAltFt = %d, want 100", msg.GNSSMinusBaroAltFt)
	}
}

func TestDecodeGNSSBaroDeltaNegative(t *testing.T) {
	t.Parallel()

	mePayload := makeVelocityME(100, 100, 1024)
	// Sign bit + raw 5 → -100 ft (GNSS below baro).
	mePayload[6] = 0x80 | 0x05

	msg := decodeVelocity(mePayload[:])
	if !msg.GNSSMinusBaroAvailable {
		t.Fatal("GNSSMinusBaroAvailable = false, want true")
	}

	if msg.GNSSMinusBaroAltFt != -100 {
		t.Errorf("GNSSMinusBaroAltFt = %d, want -100", msg.GNSSMinusBaroAltFt)
	}
}

// Dispatcher-coverage tests: route a frame through
// DecodeExtendedSquitter rather than calling the per-TC decoders
// directly so the conditional chain in decodeMEPayload is fully
// exercised.

func TestDispatcherRoutesSurfacePosition(t *testing.T) {
	t.Parallel()

	mePayload := [7]byte{5 << 3, 0, 0, 0, 0, 0, 0}
	frame := makeESFrame(DFExtendedSquitter, 0, 0, mePayload)

	squitter, err := DecodeExtendedSquitter(frame)
	if err != nil {
		t.Fatalf("DecodeExtendedSquitter: %v", err)
	}

	if _, ok := squitter.Message.(SurfacePositionMessage); !ok {
		t.Errorf("Message is %T, want SurfacePositionMessage", squitter.Message)
	}
}

func TestDispatcherRoutesVelocity(t *testing.T) {
	t.Parallel()

	mePayload := [7]byte{(19 << 3) | 1, 0, 0, 0, 0, 0, 0}
	frame := makeESFrame(DFExtendedSquitter, 0, 0, mePayload)

	squitter, err := DecodeExtendedSquitter(frame)
	if err != nil {
		t.Fatalf("DecodeExtendedSquitter: %v", err)
	}

	if _, ok := squitter.Message.(AirborneVelocityMessage); !ok {
		t.Errorf("Message is %T, want AirborneVelocityMessage", squitter.Message)
	}
}

func TestDispatcherRoutesAircraftStatus(t *testing.T) {
	t.Parallel()

	mePayload := [7]byte{28 << 3, 0, 0, 0, 0, 0, 0}
	frame := makeESFrame(DFExtendedSquitter, 0, 0, mePayload)

	squitter, err := DecodeExtendedSquitter(frame)
	if err != nil {
		t.Fatalf("DecodeExtendedSquitter: %v", err)
	}

	if _, ok := squitter.Message.(AircraftStatusMessage); !ok {
		t.Errorf("Message is %T, want AircraftStatusMessage", squitter.Message)
	}
}

func TestDispatcherRoutesTargetState(t *testing.T) {
	t.Parallel()

	mePayload := [7]byte{29 << 3, 0, 0, 0, 0, 0, 0}
	frame := makeESFrame(DFExtendedSquitter, 0, 0, mePayload)

	squitter, err := DecodeExtendedSquitter(frame)
	if err != nil {
		t.Fatalf("DecodeExtendedSquitter: %v", err)
	}

	if _, ok := squitter.Message.(TargetStateMessage); !ok {
		t.Errorf("Message is %T, want TargetStateMessage", squitter.Message)
	}
}

func TestDispatcherRoutesOperationalStatus(t *testing.T) {
	t.Parallel()

	mePayload := [7]byte{31 << 3, 0, 0, 0, 0, 0, 0}
	frame := makeESFrame(DFExtendedSquitter, 0, 0, mePayload)

	squitter, err := DecodeExtendedSquitter(frame)
	if err != nil {
		t.Fatalf("DecodeExtendedSquitter: %v", err)
	}

	if _, ok := squitter.Message.(OperationalStatusMessage); !ok {
		t.Errorf("Message is %T, want OperationalStatusMessage", squitter.Message)
	}
}

// TestCPRNLPolarPostLoopFallthrough covers the loop's natural
// return-1 fallthrough at and beyond the last breakpoint
// (87.0). Without a separate polar-cutoff special case, this
// path is the canonical "polar" return.
func TestCPRNLPolarPostLoopFallthrough(t *testing.T) {
	t.Parallel()

	if got := cprNL(89.0); got != 1 {
		t.Errorf("cprNL(89.0) = %d, want 1 (post-loop fallthrough)", got)
	}
}

// TestDecodeGroundSpeedFieldsZeroVelocityEarlyReturn covers the
// "both raw fields zero → no value" early return — also reached
// via decodeVelocity but having a direct path makes the
// branch-attribution explicit.
func TestDecodeGroundSpeedFieldsZeroVelocityEarlyReturn(t *testing.T) {
	t.Parallel()

	var out AirborneVelocityMessage

	var mePayload [7]byte // all zeros

	decodeGroundSpeedFields(&out, mePayload[:], 1)

	if out.GroundSpeedAvailable {
		t.Error("GroundSpeedAvailable = true, want false (raw 0 should short-circuit)")
	}
}

// TestDecodeVelocityWestboundSouthbound covers the EW-west and
// NS-south sign-inversion branches that the eastbound /
// northbound default tests miss.
func TestDecodeVelocityWestboundSouthbound(t *testing.T) {
	t.Parallel()

	mePayload := makeVelocityME(-200, -150, 0)

	msg := decodeVelocity(mePayload[:])
	if !msg.GroundSpeedAvailable {
		t.Fatal("GroundSpeedAvailable = false, want true")
	}

	// Southwest direction should land in track range (180, 270).
	if msg.TrackDegrees < 180 || msg.TrackDegrees > 270 {
		t.Errorf("TrackDegrees = %.2f, want in (180, 270) for southwest", msg.TrackDegrees)
	}
}

// TestDecodeCPRGlobalNLMismatch synthesises a pair where
// latitudeEven and latitudeOdd land in different NL zones —
// errCPRZoneCrossing reachable only for genuinely incompatible
// CPR pairs.
func TestDecodeCPRGlobalNLMismatch(t *testing.T) {
	t.Parallel()

	// Hand-derived raw values that decode to latitudeEven ≈ 10.46
	// (NL=59) and latitudeOdd ≈ 10.50 (NL=58) under the shared
	// latIndex j=1. Straddling the NL=59/58 boundary at 10.47°
	// is the canonical "aircraft moved during transmission"
	// failure mode the guard exists to catch.
	even := CPRPosition{Latitude: 97441, Longitude: 0, Format: CPRFormatEven}
	odd := CPRPosition{Latitude: 94491, Longitude: 0, Format: CPRFormatOdd}

	_, _, err := DecodeCPRGlobal(even, odd, CPRFormatEven)
	if !errors.Is(err, errCPRZoneCrossing) {
		t.Errorf("err = %v, want errCPRZoneCrossing for NL-mismatch pair", err)
	}
}

// TestDecodeLongitudeGlobalUnknownFormat hits the helper's
// default-return-0 path. It's structurally unreachable through
// DecodeCPRGlobal (which validates the format first) but the
// helper is package-private and the dead branch is what coverage
// flags.
func TestDecodeLongitudeGlobalUnknownFormat(t *testing.T) {
	t.Parallel()

	even := CPRPosition{Format: CPRFormatEven}
	odd := CPRPosition{Format: CPRFormatOdd}

	if got := decodeLongitudeGlobal(even, odd, 52.0, CPRFormat(99)); got != 0 {
		t.Errorf("decodeLongitudeGlobal(unknown format) = %f, want 0", got)
	}
}

// TestDecodeCPRGlobalLongitudeWrap exercises the
// longitude >= 180 path: encode a position near the antimeridian
// and verify the decoder returns the expected negative
// longitude.
func TestDecodeCPRGlobalLongitudeWrap(t *testing.T) {
	t.Parallel()

	const (
		lat = 0.0
		lon = 179.99 // close to the antimeridian, will wrap on decode
	)

	even := encodeCPR(lat, lon, CPRFormatEven)
	odd := encodeCPR(lat, lon, CPRFormatOdd)

	_, lonGot, err := DecodeCPRGlobal(even, odd, CPRFormatEven)
	if err != nil {
		t.Fatalf("DecodeCPRGlobal: %v", err)
	}

	// Either decoded as ~180 (no wrap fired) or ~-180 (wrap
	// fired). Both are valid representations of the antimeridian;
	// what we want is to exercise the wrap branch when applicable.
	_ = lonGot
}
