package modes

import (
	"errors"
	"testing"
)

// makeCommBFrame synthesises a DF 20 / DF 21 frame with the
// given header fields, 13-bit AC/ID payload, and 7-byte MB
// field. AppendCRC24 fills the AP tail (parity-overlay scheme
// not reproducible without the addressed ICAO; tests pass the
// ICAO as a separate argument).
func makeCommBFrame(
	downlinkFormat DownlinkFormat,
	flightStatus FlightStatus,
	downlinkRequest, utilityMessage uint8,
	payload13 uint16,
	commBPayload [7]byte,
) Frame {
	const dfShift = 3

	body := make([]byte, 0, 4+len(commBPayload))
	body = append(body,
		byte(downlinkFormat<<dfShift)|(byte(flightStatus)&0x07),
		(downlinkRequest<<3)|((utilityMessage>>3)&0x07),
		((utilityMessage&0x07)<<5)|byte(payload13>>8&0x1F),
		byte(payload13&0xFF),
	)
	body = append(body, commBPayload[:]...)

	return Frame(AppendCRC24(body))
}

func TestDecodeCommBAltitudeRoundTrip(t *testing.T) {
	t.Parallel()

	const wantICAO ICAO = 0x40621D

	wantMB := [7]byte{0x20, 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC}

	frame := makeCommBFrame(
		DFCommBAltitude,
		FlightStatusAirborne,
		0, 0,
		makeAltitudeQ1(35_000),
		wantMB,
	)

	reply, err := DecodeCommBAltitude(frame, wantICAO)
	if err != nil {
		t.Fatalf("DecodeCommBAltitude: %v", err)
	}

	if reply.AltitudeFeet != 35_000 {
		t.Errorf("AltitudeFeet = %d, want 35_000", reply.AltitudeFeet)
	}

	if reply.MB != wantMB {
		t.Errorf("MB = %x, want %x", reply.MB, wantMB)
	}
}

func TestDecodeCommBIdentityRoundTrip(t *testing.T) {
	t.Parallel()

	const wantICAO ICAO = 0xABCDEF

	wantMB := [7]byte{0x60, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	frame := makeCommBFrame(
		DFCommBIdentity,
		FlightStatusAirborneAlert,
		0, 0,
		makeIdentityCode(7700),
		wantMB,
	)

	reply, err := DecodeCommBIdentity(frame, wantICAO)
	if err != nil {
		t.Fatalf("DecodeCommBIdentity: %v", err)
	}

	if reply.Squawk != 7700 {
		t.Errorf("Squawk = %04d, want 7700", reply.Squawk)
	}

	if reply.MB != wantMB {
		t.Errorf("MB = %x, want %x", reply.MB, wantMB)
	}
}

func TestDecodeCommBAltitudeRejectsWrongDF(t *testing.T) {
	t.Parallel()

	frame := make(Frame, LongFrameBytes)
	frame[0] = byte(DFExtendedSquitter << 3)

	if _, err := DecodeCommBAltitude(frame, 0); !errors.Is(err, ErrWrongDF) {
		t.Errorf("err = %v, want ErrWrongDF", err)
	}
}

func TestDecodeBDS20Callsign(t *testing.T) {
	t.Parallel()

	// Pack a BDS 2,0 register: code = 0x20, then 8×6-bit chars
	// for "TEST    ".
	mb := buildBDS20MB("TEST")

	got, ok := DecodeBDS20Callsign(mb)
	if !ok {
		t.Error("DecodeBDS20Callsign ok = false, want true (BDS code 0x20 set correctly)")
	}

	if got != "TEST" {
		t.Errorf("callsign = %q, want %q", got, "TEST")
	}
}

func TestDecodeBDS20CallsignWrongCodeStillDecodes(t *testing.T) {
	t.Parallel()

	mb := buildBDS20MB("HELLO")
	mb[0] = 0xFF // wrong BDS code

	got, ok := DecodeBDS20Callsign(mb)
	if ok {
		t.Error("ok = true, want false (BDS code does not match 0x20)")
	}

	// Decoder still produces a callsign — caller decides.
	if got == "" {
		t.Error("callsign = empty, want some attempt")
	}
}

func buildBDS20MB(callsign string) [7]byte {
	const (
		callsignChars = 8
		bitsPerChar   = 6
		meBits        = 56
	)

	encode := func(char byte) uint64 {
		switch {
		case char >= 'A' && char <= 'Z':
			return uint64(char - 'A' + 1)
		case char >= '0' && char <= '9':
			return uint64(char - '0' + 48)
		case char == ' ':
			return 32
		}

		return 0
	}

	padded := make([]byte, callsignChars)

	for index := range padded {
		padded[index] = ' '
	}

	for index := 0; index < callsignChars && index < len(callsign); index++ {
		padded[index] = callsign[index]
	}

	meWord := uint64(0x20) << (meBits - 8) //nolint:mnd // BDS code prefix.
	for index, char := range padded {
		shift := uint(meBits - 8 - (index+1)*bitsPerChar)
		meWord |= encode(char) << shift
	}

	var out [7]byte
	for index := range out {
		out[index] = byte((meWord >> uint(meBits-8-index*8)) & 0xFF)
	}

	return out
}
