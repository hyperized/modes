package modes

import (
	"errors"
	"testing"
)

// makeSurveillanceFrame synthesises a 7-byte DF 4 or DF 5 frame
// with the given header fields and 13-bit payload. The trailing
// 24-bit AP is filled by AppendCRC24, mirroring an unsolicited
// path; tests pass the addressed ICAO as a separate argument
// since the parity-overlay scheme isn't reproducible from CRC24
// alone.
func makeSurveillanceFrame(
	downlinkFormat DownlinkFormat,
	flightStatus FlightStatus,
	downlinkRequest, utilityMessage uint8,
	payload13 uint16,
) Frame {
	const dfShift = 3

	body := []byte{
		byte(downlinkFormat<<dfShift) | (byte(flightStatus) & 0x07),
		(downlinkRequest << 3) | ((utilityMessage >> 3) & 0x07),
		((utilityMessage & 0x07) << 5) | byte(payload13>>8&0x1F),
		byte(payload13 & 0xFF),
	}

	return Frame(AppendCRC24(body))
}

func TestDecodeSurveillanceAltitudeRoundTrip(t *testing.T) {
	t.Parallel()

	const wantICAO ICAO = 0x40621D

	altitudeCode := makeAltitudeQ1(35_000)
	frame := makeSurveillanceFrame(
		DFSurveillanceAlt,
		FlightStatusAirborne,
		0b10101,  // DR
		0b110011, // UM
		altitudeCode,
	)

	reply, err := DecodeSurveillanceAltitude(frame, wantICAO)
	if err != nil {
		t.Fatalf("DecodeSurveillanceAltitude: %v", err)
	}

	if reply.ICAO != wantICAO {
		t.Errorf("ICAO = %#06x, want %#06x", reply.ICAO, wantICAO)
	}

	if reply.FlightStatus != FlightStatusAirborne {
		t.Errorf("FlightStatus = %d, want %d", reply.FlightStatus, FlightStatusAirborne)
	}

	if reply.DownlinkRequest != 0b10101 {
		t.Errorf("DownlinkRequest = %#05b, want %#05b", reply.DownlinkRequest, 0b10101)
	}

	if reply.UtilityMessage != 0b110011 {
		t.Errorf("UtilityMessage = %#06b, want %#06b", reply.UtilityMessage, 0b110011)
	}

	if reply.AltitudeError != nil {
		t.Errorf("AltitudeError = %v, want nil", reply.AltitudeError)
	}

	if reply.AltitudeFeet != 35_000 {
		t.Errorf("AltitudeFeet = %d, want 35_000", reply.AltitudeFeet)
	}
}

func TestDecodeSurveillanceAltitudeFlagsAltitudeError(t *testing.T) {
	t.Parallel()

	// M=1 path: AltitudeFeet returns ErrAltitudeMSet but the
	// surveillance decode itself succeeds — the consumer can
	// still see flight status / DR / UM even when altitude is
	// not decodable.
	const mBitInPayload uint16 = 1 << 6

	frame := makeSurveillanceFrame(DFSurveillanceAlt, FlightStatusOnGround, 0, 0, mBitInPayload)

	reply, err := DecodeSurveillanceAltitude(frame, 0)
	if err != nil {
		t.Fatalf("DecodeSurveillanceAltitude: %v", err)
	}

	if !errors.Is(reply.AltitudeError, ErrAltitudeMSet) {
		t.Errorf("AltitudeError = %v, want ErrAltitudeMSet", reply.AltitudeError)
	}
}

func TestDecodeSurveillanceIdentityRoundTrip(t *testing.T) {
	t.Parallel()

	const wantICAO ICAO = 0xABCDEF

	frame := makeSurveillanceFrame(
		DFSurveillanceID,
		FlightStatusAirborneAlert,
		0, 0,
		makeIdentityCode(7700),
	)

	reply, err := DecodeSurveillanceIdentity(frame, wantICAO)
	if err != nil {
		t.Fatalf("DecodeSurveillanceIdentity: %v", err)
	}

	if reply.Squawk != 7700 {
		t.Errorf("Squawk = %04d, want 7700", reply.Squawk)
	}

	if reply.FlightStatus != FlightStatusAirborneAlert {
		t.Errorf("FlightStatus = %d, want %d", reply.FlightStatus, FlightStatusAirborneAlert)
	}
}

func TestDecodeSurveillanceAltitudeRejectsWrongDF(t *testing.T) {
	t.Parallel()

	frame := makeSurveillanceFrame(DFSurveillanceID, 0, 0, 0, 0)
	if _, err := DecodeSurveillanceAltitude(frame, 0); !errors.Is(err, ErrWrongDF) {
		t.Errorf("err = %v, want ErrWrongDF", err)
	}
}

func TestDecodeSurveillanceIdentityRejectsWrongDF(t *testing.T) {
	t.Parallel()

	frame := makeSurveillanceFrame(DFSurveillanceAlt, 0, 0, 0, 0)
	if _, err := DecodeSurveillanceIdentity(frame, 0); !errors.Is(err, ErrWrongDF) {
		t.Errorf("err = %v, want ErrWrongDF", err)
	}
}

func TestDecodeSurveillanceAltitudeRejectsShortFrame(t *testing.T) {
	t.Parallel()

	frame := Frame{byte(DFSurveillanceAlt << 3)}
	if _, err := DecodeSurveillanceAltitude(frame, 0); !errors.Is(err, ErrFrameTooShort) {
		t.Errorf("err = %v, want ErrFrameTooShort", err)
	}
}

func TestDecodeSurveillanceIdentityRejectsShortFrame(t *testing.T) {
	t.Parallel()

	frame := Frame{byte(DFSurveillanceID << 3)}
	if _, err := DecodeSurveillanceIdentity(frame, 0); !errors.Is(err, ErrFrameTooShort) {
		t.Errorf("err = %v, want ErrFrameTooShort", err)
	}
}
