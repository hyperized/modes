package modes

import (
	"errors"
	"testing"
)

// makeACASShortFrame synthesises a DF 0 frame with the given
// prefix fields and altitude code, with the AP filled by
// AppendCRC24 (i.e. unsolicited self-cancelling parity, since
// the address-overlay scheme isn't reproducible without ICAO
// knowledge).
func makeACASShortFrame(
	verticalStatus VerticalStatus,
	crossLink CrossLinkCapability,
	sensitivityLevel SensitivityLevel,
	replyInformation ReplyInformation,
	altitudeCode uint16,
) Frame {
	return Frame(AppendCRC24(buildACASPrefixBytes(
		DFShortAirAir, verticalStatus, crossLink, sensitivityLevel, replyInformation, altitudeCode,
	)))
}

// makeACASLongFrame synthesises a DF 16 frame with the given
// prefix fields, altitude code, and 7-byte MV payload.
func makeACASLongFrame(
	verticalStatus VerticalStatus,
	sensitivityLevel SensitivityLevel,
	replyInformation ReplyInformation,
	altitudeCode uint16,
	messageVehicle [7]byte,
) Frame {
	body := buildACASPrefixBytes(
		DFLongAirAir, verticalStatus, CrossLinkUnsupported, sensitivityLevel, replyInformation, altitudeCode,
	)
	body = append(body, messageVehicle[:]...)

	return Frame(AppendCRC24(body))
}

func buildACASPrefixBytes(
	downlinkFormat DownlinkFormat,
	verticalStatus VerticalStatus,
	crossLink CrossLinkCapability,
	sensitivityLevel SensitivityLevel,
	replyInformation ReplyInformation,
	altitudeCode uint16,
) []byte {
	const (
		dfShift  uint8 = 3
		vsShift  uint8 = 2
		ccShift  uint8 = 1
		slShift  uint8 = 5
		riShift  uint8 = 1
		riLowPos uint8 = 7
	)

	byte0 := byte(downlinkFormat<<dfShift) |
		(byte(verticalStatus) << vsShift) |
		(byte(crossLink) << ccShift)

	byte1 := byte(sensitivityLevel<<slShift) | byte(replyInformation>>riShift)

	byte2 := byte(replyInformation&0x01)<<riLowPos |
		byte(altitudeCode>>8&0x1F)

	byte3 := byte(altitudeCode & 0xFF)

	return []byte{byte0, byte1, byte2, byte3}
}

func TestDecodeACASShortReplyRoundTrip(t *testing.T) {
	t.Parallel()

	const wantICAO ICAO = 0x484755

	frame := makeACASShortFrame(
		VerticalStatusAirborne,
		CrossLinkSupported,
		SensitivityLevel(5),
		ReplyInformation(0b1001),
		makeAltitudeQ1(35_000),
	)

	reply, err := DecodeACASShortReply(frame, wantICAO)
	if err != nil {
		t.Fatalf("DecodeACASShortReply: %v", err)
	}

	if reply.ICAO != wantICAO {
		t.Errorf("ICAO = %#06x, want %#06x", reply.ICAO, wantICAO)
	}

	if reply.VerticalStatus != VerticalStatusAirborne {
		t.Errorf("VerticalStatus = %d, want %d", reply.VerticalStatus, VerticalStatusAirborne)
	}

	if reply.CrossLinkCapability != CrossLinkSupported {
		t.Errorf("CrossLinkCapability = %d, want %d", reply.CrossLinkCapability, CrossLinkSupported)
	}

	if reply.SensitivityLevel != 5 {
		t.Errorf("SensitivityLevel = %d, want 5", reply.SensitivityLevel)
	}

	if reply.ReplyInformation != 0b1001 {
		t.Errorf("ReplyInformation = %#04b, want %#04b", reply.ReplyInformation, 0b1001)
	}

	if reply.AltitudeError != nil {
		t.Errorf("AltitudeError = %v, want nil", reply.AltitudeError)
	}

	if reply.AltitudeFeet != 35_000 {
		t.Errorf("AltitudeFeet = %d, want 35_000", reply.AltitudeFeet)
	}
}

func TestDecodeACASLongReplyRoundTrip(t *testing.T) {
	t.Parallel()

	const wantICAO ICAO = 0xABCDEF

	wantMV := [7]byte{0x30, 0xA5, 0x00, 0x00, 0x00, 0x00, 0x00}

	frame := makeACASLongFrame(
		VerticalStatusOnGround,
		SensitivityLevel(2),
		ReplyInformation(0b0011),
		makeAltitudeQ1(1500),
		wantMV,
	)

	reply, err := DecodeACASLongReply(frame, wantICAO)
	if err != nil {
		t.Fatalf("DecodeACASLongReply: %v", err)
	}

	if reply.ICAO != wantICAO {
		t.Errorf("ICAO = %#06x, want %#06x", reply.ICAO, wantICAO)
	}

	if reply.VerticalStatus != VerticalStatusOnGround {
		t.Errorf("VerticalStatus = %d, want %d", reply.VerticalStatus, VerticalStatusOnGround)
	}

	if reply.MV != wantMV {
		t.Errorf("MV = %x, want %x", reply.MV, wantMV)
	}

	if reply.AltitudeFeet != 1500 {
		t.Errorf("AltitudeFeet = %d, want 1500", reply.AltitudeFeet)
	}
}

func TestDecodeACASShortReplyRejectsWrongDF(t *testing.T) {
	t.Parallel()

	frame := Frame{byte(DFAllCallReply << 3), 0, 0, 0, 0, 0, 0}
	if _, err := DecodeACASShortReply(frame, 0); !errors.Is(err, ErrWrongDF) {
		t.Errorf("err = %v, want ErrWrongDF", err)
	}
}

func TestDecodeACASLongReplyRejectsWrongDF(t *testing.T) {
	t.Parallel()

	frame := make(Frame, LongFrameBytes)
	frame[0] = byte(DFExtendedSquitter << 3)

	if _, err := DecodeACASLongReply(frame, 0); !errors.Is(err, ErrWrongDF) {
		t.Errorf("err = %v, want ErrWrongDF", err)
	}
}

func TestDecodeACASShortReplyRejectsShortFrame(t *testing.T) {
	t.Parallel()

	frame := Frame{byte(DFShortAirAir << 3)}
	if _, err := DecodeACASShortReply(frame, 0); !errors.Is(err, ErrFrameTooShort) {
		t.Errorf("err = %v, want ErrFrameTooShort", err)
	}
}

func TestDecodeACASLongReplyRejectsShortFrame(t *testing.T) {
	t.Parallel()

	frame := Frame{byte(DFLongAirAir << 3)}
	if _, err := DecodeACASLongReply(frame, 0); !errors.Is(err, ErrFrameTooShort) {
		t.Errorf("err = %v, want ErrFrameTooShort", err)
	}
}
