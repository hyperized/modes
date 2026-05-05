package modes

import (
	"errors"
	"testing"
)

// makeAirbornePositionME synthesises a TC 9..18 ME payload with
// the given altitude (in feet, Q=1 binary form) and an explicit
// 17-bit CPR latitude / longitude pair.
//
//nolint:gosec // bounded by test inputs (altitude well under 50_000 ft, raw CPR < 2^17).
func makeAirbornePositionME(typeCode TypeCode, altitudeFeet int, format CPRFormat, latCPR, lonCPR uint32) [7]byte {
	const (
		altMultiplier        = 25
		altOffset            = -1000
		qBitMask      uint16 = 1 << 4
		topMask       uint16 = 0xFE0
		botMask       uint16 = 0x00F
	)

	n := uint16((altitudeFeet - altOffset) / altMultiplier)

	altCode := ((n << 1) & topMask) | (n & botMask) | qBitMask //nolint:mnd // 12-bit AC layout reverse.

	formatBit := byte(0)
	if format == CPRFormatOdd {
		formatBit = 1
	}

	var mePayload [7]byte

	mePayload[0] = byte(typeCode << 3) // SS=0, SAF=0
	mePayload[1] = byte(altCode >> 4)
	mePayload[2] = byte((altCode&0x0F)<<4) | (formatBit << 2) | byte((latCPR>>15)&0x03)
	mePayload[3] = byte((latCPR >> 7) & 0xFF)
	mePayload[4] = byte((latCPR&0x7F)<<1) | byte((lonCPR>>16)&0x01)
	mePayload[5] = byte((lonCPR >> 8) & 0xFF)
	mePayload[6] = byte(lonCPR & 0xFF)

	return mePayload
}

func TestDecodeAirbornePositionRoundTrip(t *testing.T) {
	t.Parallel()

	const (
		typeCode     TypeCode = 11
		wantAltitude          = 35_000
		wantICAO     ICAO     = 0x484755
	)

	cprPos := encodeCPR(52.3676, 4.9041, CPRFormatEven)

	mePayload := makeAirbornePositionME(typeCode, wantAltitude, cprPos.Format, cprPos.Latitude, cprPos.Longitude)
	frame := makeESFrame(DFExtendedSquitter, 0, wantICAO, mePayload)

	squitter, err := DecodeExtendedSquitter(frame)
	if err != nil {
		t.Fatalf("DecodeExtendedSquitter: %v", err)
	}

	pos, ok := squitter.Message.(AirbornePositionMessage)
	if !ok {
		t.Fatalf("Message is %T, want AirbornePositionMessage", squitter.Message)
	}

	if pos.AltitudeError != nil {
		t.Errorf("AltitudeError = %v, want nil", pos.AltitudeError)
	}

	if pos.AltitudeFeet != wantAltitude {
		t.Errorf("AltitudeFeet = %d, want %d", pos.AltitudeFeet, wantAltitude)
	}

	if pos.CPR.Format != CPRFormatEven {
		t.Errorf("CPR.Format = %d, want %d", pos.CPR.Format, CPRFormatEven)
	}

	if pos.CPR.Latitude != cprPos.Latitude {
		t.Errorf("CPR.Latitude = %d, want %d", pos.CPR.Latitude, cprPos.Latitude)
	}

	if pos.CPR.Longitude != cprPos.Longitude {
		t.Errorf("CPR.Longitude = %d, want %d", pos.CPR.Longitude, cprPos.Longitude)
	}

	if pos.IsGNSSAltitude {
		t.Error("IsGNSSAltitude = true, want false (TC 11 is barometric)")
	}
}

func TestDecodeAirbornePositionGNSSReturnsAltitudeError(t *testing.T) {
	t.Parallel()

	// TC 20..22 carry GNSS altitude — the decoder surfaces
	// ErrGNSSAltitudeUnsupported on the AltitudeError field
	// while still populating the CPR / format structural data.
	mePayload := makeAirbornePositionME(20, 35_000, CPRFormatEven, 0, 0)
	frame := makeESFrame(DFExtendedSquitter, 0, 0x123456, mePayload)

	squitter, err := DecodeExtendedSquitter(frame)
	if err != nil {
		t.Fatalf("DecodeExtendedSquitter: %v", err)
	}

	pos, ok := squitter.Message.(AirbornePositionMessage)
	if !ok {
		t.Fatalf("Message is %T, want AirbornePositionMessage", squitter.Message)
	}

	if !errors.Is(pos.AltitudeError, ErrGNSSAltitudeUnsupported) {
		t.Errorf("AltitudeError = %v, want ErrGNSSAltitudeUnsupported", pos.AltitudeError)
	}

	if !pos.IsGNSSAltitude {
		t.Error("IsGNSSAltitude = false, want true (TC 20 is GNSS)")
	}
}

// TestDecodeAirbornePositionAltitudeQ0Gillham hits the Q=0
// branch — the modern decoder doesn't handle Gillham yet, so it
// must surface ErrGillhamUnsupported on the altitude field.
func TestDecodeAirbornePositionAltitudeQ0Gillham(t *testing.T) {
	t.Parallel()

	// Build an ME payload with Q=0 in the altitude field. The
	// 12-bit altitude code is bytes 1-2 high nibble; clearing
	// bit 4 (Q) selects Gillham.
	var mePayload [7]byte

	mePayload[0] = byte(11 << 3) // TC 11
	mePayload[1] = 0xAA          // arbitrary high byte
	mePayload[2] = 0xA0          // Q bit (position 4 of altitude code) is 0

	frame := makeESFrame(DFExtendedSquitter, 0, 0xABCDEF, mePayload)

	squitter, err := DecodeExtendedSquitter(frame)
	if err != nil {
		t.Fatalf("DecodeExtendedSquitter: %v", err)
	}

	pos, ok := squitter.Message.(AirbornePositionMessage)
	if !ok {
		t.Fatalf("Message is %T, want AirbornePositionMessage", squitter.Message)
	}

	if !errors.Is(pos.AltitudeError, ErrGillhamUnsupported) {
		t.Errorf("AltitudeError = %v, want ErrGillhamUnsupported", pos.AltitudeError)
	}
}

// TestDecodeAirbornePositionTimeFlagBit covers the TimeFlag /
// SAF / SS extraction paths that the round-trip test happens to
// leave at zero.
func TestDecodeAirbornePositionTimeFlagBit(t *testing.T) {
	t.Parallel()

	mePayload := makeAirbornePositionME(11, 0, CPRFormatEven, 0, 0)
	mePayload[0] |= 0x05 // SS = 0b10 (TemporaryAlert), SAF = 1
	mePayload[2] |= 0x08 // T bit set

	frame := makeESFrame(DFExtendedSquitter, 0, 0x111111, mePayload)

	squitter, err := DecodeExtendedSquitter(frame)
	if err != nil {
		t.Fatalf("DecodeExtendedSquitter: %v", err)
	}

	pos, ok := squitter.Message.(AirbornePositionMessage)
	if !ok {
		t.Fatalf("Message is %T, want AirbornePositionMessage", squitter.Message)
	}

	if pos.SurveillanceStatus != SurveillanceStatusTemporaryAlert {
		t.Errorf("SurveillanceStatus = %d, want %d", pos.SurveillanceStatus, SurveillanceStatusTemporaryAlert)
	}

	if !pos.SingleAntennaFlag {
		t.Error("SingleAntennaFlag = false, want true")
	}

	if !pos.TimeFlag {
		t.Error("TimeFlag = false, want true")
	}
}
