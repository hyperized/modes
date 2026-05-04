package modes

import (
	"errors"
	"fmt"
)

// DF 11 — All-Call Reply
// =====================
//
// ICAO Annex 10 Vol IV §3.1.2.5.2.1.
//
// The Mode S all-call is the broadcast interrogation Mode A/C/S
// radars emit to roll-call every transponder in beam. Every
// Mode S transponder responds with a DF 11 reply carrying its CA
// (Capability) and AA (Announced Address — the ICAO).
//
// Wire layout (56 bits = 7 bytes):
//
//	bits 0-4:   DF (5 bits, value 11)
//	bits 5-7:   CA (3 bits, capability code)
//	bits 8-31:  AA (24 bits, ICAO address)
//	bits 32-55: PI (24 bits, parity / interrogator ID overlay)
//
// The PI field is parity overlaid with the Interrogator
// Identifier (II 1..15) the radar selected; for unsolicited or
// roll-call replies (II = 0) the PI is the plain CRC and the
// receiver's CRC residual is zero.
//
// CapabilityCode interpretation
// -----------------------------
// CA values are documented in §3.1.2.5.2.2.1:
//
//	0  Level 1 transponder (no comm-A/B/C/D capability)
//	1-3 reserved
//	4  Level 2+ transponder, on the ground
//	5  Level 2+ transponder, airborne
//	6  Level 2+ transponder, on ground or airborne (CA can't tell)
//	7  Downlink Format 24-31 transponder (Comm-D ELM capable)

// CapabilityCode is the 3-bit CA field at bits 5-7 of DF 11.
type CapabilityCode uint8

// Documented capability values per ICAO Annex 10 Vol IV §3.1.2.5.2.2.1.
const (
	CapabilityLevel1                CapabilityCode = 0
	CapabilityLevel2OnGround        CapabilityCode = 4
	CapabilityLevel2Airborne        CapabilityCode = 5
	CapabilityLevel2EitherState     CapabilityCode = 6
	CapabilityCommDExtendedLengthOK CapabilityCode = 7
)

// AllCallReply is the decoded form of a DF 11 frame. The
// Interrogator field carries the radar identifier the
// transponder is replying to (0 for unsolicited / roll-call
// replies) — derived from the CRC residual rather than carried
// in the message body, so callers must pass it in alongside the
// frame bytes.
type AllCallReply struct {
	Capability    CapabilityCode
	ICAO          ICAO
	Interrogator  uint8
	IsUnsolicited bool
}

// errFrameTooShort is the static sentinel for short-frame decode
// errors. err113 forbids ad-hoc errors.New from fmt.Errorf, so
// individual DF decoders wrap this with %w.
var errFrameTooShort = errors.New("modes: frame shorter than DF requires")

// errWrongDF is the static sentinel for "this decoder was handed
// a frame whose DF doesn't match what the decoder handles".
var errWrongDF = errors.New("modes: wrong downlink format for decoder")

// DecodeAllCallReply parses a DF 11 frame and returns the typed
// reply. The interrogator field is derived from the CRC residual,
// which the caller computed at validation time; pass it as
// crcResidual.
//
// Returns errWrongDF if the frame's DF isn't 11, errFrameTooShort
// if the frame isn't ShortFrameBytes long.
func DecodeAllCallReply(frame Frame, crcResidual uint32) (AllCallReply, error) {
	if got := frame.DF(); got != DFAllCallReply {
		return AllCallReply{}, fmt.Errorf("%w: have DF %d, want %d", errWrongDF, got, DFAllCallReply)
	}

	if len(frame) != ShortFrameBytes {
		return AllCallReply{}, fmt.Errorf("%w: have %d bytes, want %d for DF 11",
			errFrameTooShort, len(frame), ShortFrameBytes)
	}

	const (
		// CA occupies the bottom 3 bits of byte 0; the top 5 bits
		// hold the DF.
		capabilityMask uint8 = 0x07

		// AA bytes are 1, 2, 3 of the frame.
		aaOffset = 1

		// The unsolicited interrogator ID is 0; values 1..15
		// indicate a specific Mode S radar replied to a roll-call.
		interrogatorMask uint32 = 0xF
	)

	return AllCallReply{
		Capability:    CapabilityCode(frame[0] & capabilityMask),
		ICAO:          extractICAO24(frame, aaOffset),
		Interrogator:  uint8(crcResidual & interrogatorMask), //nolint:gosec // 4-bit mask.
		IsUnsolicited: crcResidual == 0,
	}, nil
}
