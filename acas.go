package modes

import "fmt"

// ACAS air-to-air replies (DF 0, DF 16)
// =====================================
//
// ICAO Annex 10 Vol IV §3.1.2.8.2 (DF 0 short) and §3.1.2.8.3
// (DF 16 long). Mode S-equipped aircraft running an ACAS / TCAS
// implementation reply to air-to-air interrogations from nearby
// aircraft. The short reply is functionally identical to a
// surveillance altitude reply but with ACAS-specific status bits
// in place of the surveillance FS/DR/UM header. The long reply
// adds a 56-bit MV field carrying the TCAS coordination payload
// (resolution advisories, intent, etc.).
//
// Wire layout of the 32-bit prefix shared by DF 0 and DF 16:
//
//	bit 31..27 = DF (5 bits)
//	bit 26     = VS (1 bit, vertical status: 0 = airborne, 1 = on ground)
//	bit 25     = CC (1 bit, DF 0 only — cross-link capability)
//	bit 24     = spare
//	bit 23..21 = SL (3 bits, sensitivity level report)
//	bit 20..19 = spare
//	bit 18..15 = RI (4 bits, reply information / max airspeed)
//	bit 14..13 = spare
//	bit 12..0  = AC (13 bits, altitude code)
//
// DF 0 stops there (24-bit AP follows). DF 16 adds 56 more bits
// of MV before the AP.

// VerticalStatus encodes the aircraft's reported on-ground vs
// airborne state in the VS bit.
type VerticalStatus uint8

// Documented VS values per §3.1.2.8.2.1.
const (
	VerticalStatusAirborne VerticalStatus = 0
	VerticalStatusOnGround VerticalStatus = 1
)

// SensitivityLevel is the 3-bit SL field — the ACAS "sensitivity
// level" the transponder is operating at. 0 means TCAS is
// inhibited; 1..7 indicate increasingly sensitive RA thresholds.
type SensitivityLevel uint8

// ReplyInformation is the 4-bit RI field. Encodes either the
// transponder's max airspeed (RI 0..7 with TCAS-equipped 8..15
// flagging the equipage). Per §3.1.2.8.2.2.
type ReplyInformation uint8

// CrossLinkCapability is the 1-bit CC field on DF 0. When set,
// the transponder supports cross-link interrogation handling.
// DF 16 reuses the bit position as a spare; DF 0 callers should
// honour it.
type CrossLinkCapability uint8

// Documented CC values.
const (
	CrossLinkUnsupported CrossLinkCapability = 0
	CrossLinkSupported   CrossLinkCapability = 1
)

// ACASShortReply is the decoded form of a DF 0 frame.
type ACASShortReply struct {
	VerticalStatus      VerticalStatus
	CrossLinkCapability CrossLinkCapability
	SensitivityLevel    SensitivityLevel
	ReplyInformation    ReplyInformation
	AltitudeFeet        int
	AltitudeError       error
	ICAO                ICAO
}

// ACASLongReply is the decoded form of a DF 16 frame. The MV
// field carries the TCAS coordination payload (resolution
// advisory message); we expose it as raw bytes here and leave
// MV-subfield decoding to a downstream consumer that needs RA
// telemetry — most receivers care about the prefix fields and
// can ignore MV.
type ACASLongReply struct {
	VerticalStatus   VerticalStatus
	SensitivityLevel SensitivityLevel
	ReplyInformation ReplyInformation
	AltitudeFeet     int
	AltitudeError    error

	// MV is the 7-byte ACAS coordination payload (bits 32..87
	// of the long frame). Independent subprotocol; decode it
	// separately if needed.
	MV [7]byte

	ICAO ICAO
}

// acasPrefix holds the bit-extracted view of the 4-byte prefix
// shared by DF 0 and DF 16.
type acasPrefix struct {
	verticalStatus      VerticalStatus
	crossLinkCapability CrossLinkCapability
	sensitivityLevel    SensitivityLevel
	replyInformation    ReplyInformation
	altitudeCode        uint16
}

// extractACASPrefix decodes the 32-bit ACAS prefix common to DF 0
// and DF 16. The CC field is meaningful only on DF 0; for DF 16
// the same bit position is a spare and the caller should ignore
// the cross-link capability.
func extractACASPrefix(frame Frame) acasPrefix {
	const (
		vsBitPos uint8 = 2
		ccBitPos uint8 = 1

		slShift uint8 = 5
		slMask  uint8 = 0x07

		// RI straddles bytes 1 and 2: the top three bits live in
		// byte 1's low nibble, the fourth bit is byte 2's MSB.
		riHighShift uint8 = 1
		riHighMask  uint8 = 0x07
		riLowShift  uint8 = 7
		riLowMask   uint8 = 0x01

		acHighShift uint16 = 8
		acMask      uint16 = 0x1FFF
	)

	verticalStatus := VerticalStatus((frame[0] >> vsBitPos) & 1)
	crossLink := CrossLinkCapability((frame[0] >> ccBitPos) & 1)

	sensitivityLevel := SensitivityLevel((frame[1] >> slShift) & slMask)

	riHigh := (frame[1] & riHighMask) << riHighShift
	riLow := (frame[2] >> riLowShift) & riLowMask
	replyInformation := ReplyInformation(riHigh | riLow)

	altitudeCode := (uint16(frame[2])<<acHighShift | uint16(frame[3])) & acMask

	return acasPrefix{
		verticalStatus:      verticalStatus,
		crossLinkCapability: crossLink,
		sensitivityLevel:    sensitivityLevel,
		replyInformation:    replyInformation,
		altitudeCode:        altitudeCode,
	}
}

// DecodeACASShortReply parses a DF 0 frame. icao is the addressed
// aircraft's ICAO recovered from the parity residual.
func DecodeACASShortReply(frame Frame, icao ICAO) (ACASShortReply, error) {
	// Length-check first: Frame.DF() panics on an empty slice, so
	// surfacing ErrFrameTooShort for the empty case keeps the
	// public Decode* surface panic-free for arbitrary input.
	if len(frame) != ShortFrameBytes {
		return ACASShortReply{}, fmt.Errorf("%w: have %d bytes, want %d for DF 0",
			ErrFrameTooShort, len(frame), ShortFrameBytes)
	}

	if got := frame.DF(); got != DFShortAirAir {
		return ACASShortReply{}, fmt.Errorf("%w: have DF %d, want %d",
			ErrWrongDF, got, DFShortAirAir)
	}

	prefix := extractACASPrefix(frame)

	altitude, err := AltitudeFeet(prefix.altitudeCode)

	return ACASShortReply{
		VerticalStatus:      prefix.verticalStatus,
		CrossLinkCapability: prefix.crossLinkCapability,
		SensitivityLevel:    prefix.sensitivityLevel,
		ReplyInformation:    prefix.replyInformation,
		AltitudeFeet:        altitude,
		AltitudeError:       err,
		ICAO:                icao,
	}, nil
}

// DecodeACASLongReply parses a DF 16 frame. icao contract matches
// DecodeACASShortReply; the MV field is exposed verbatim as a
// 7-byte payload.
func DecodeACASLongReply(frame Frame, icao ICAO) (ACASLongReply, error) {
	// Length-check first: Frame.DF() panics on an empty slice.
	if len(frame) != LongFrameBytes {
		return ACASLongReply{}, fmt.Errorf("%w: have %d bytes, want %d for DF 16",
			ErrFrameTooShort, len(frame), LongFrameBytes)
	}

	if got := frame.DF(); got != DFLongAirAir {
		return ACASLongReply{}, fmt.Errorf("%w: have DF %d, want %d",
			ErrWrongDF, got, DFLongAirAir)
	}

	prefix := extractACASPrefix(frame)

	altitude, err := AltitudeFeet(prefix.altitudeCode)

	const (
		mvOffset = 4
		mvLen    = 7
	)

	var messageVehicle [mvLen]byte

	copy(messageVehicle[:], frame[mvOffset:mvOffset+mvLen])

	return ACASLongReply{
		VerticalStatus:   prefix.verticalStatus,
		SensitivityLevel: prefix.sensitivityLevel,
		ReplyInformation: prefix.replyInformation,
		AltitudeFeet:     altitude,
		AltitudeError:    err,
		MV:               messageVehicle,
		ICAO:             icao,
	}, nil
}
