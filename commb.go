package modes

import "fmt"

// Comm-B replies (DF 20, DF 21)
// =============================
//
// ICAO Annex 10 Vol IV §3.1.2.6.6 (DF 20 altitude) and §3.1.2.6.8
// (DF 21 identity). DF 20 / DF 21 are 14-byte frames sharing the
// 4-byte FS / DR / UM / AC-or-ID header with DF 4 / DF 5, plus a
// 56-bit Comm-B message field (MB) that carries one of many
// "B-Defined Subaddress" (BDS) register payloads:
//
//	BDS 1,0  Data Link Capability
//	BDS 1,7  Common Usage GICB Capability Report
//	BDS 2,0  Aircraft Identification (callsign)
//	BDS 3,0  ACAS Resolution Advisory
//	BDS 4,0  Selected Vertical Intention
//	BDS 4,4  Meteorological Routine Air Report
//	BDS 4,5  Meteorological Hazard Report
//	BDS 5,0  Track and Turn Report
//	BDS 6,0  Heading and Speed Report
//	... and many more
//
// The wire frame does not name which register the MB field
// belongs to — that's implicit from the ground's interrogation
// request, which the airborne receiver never sees. Callers must
// either know from context or run a heuristic "BDS detector"
// that identifies the register from MB-field patterns. This
// package exposes the raw MB so callers can do either.
//
// Two register-specific decoders ship today: BDS 2,0 (callsign)
// because it round-trips the Mode S 6-bit alphabet identically
// to the TC 1..4 ADS-B path, and BDS 3,0 (ACAS RA) because it's
// the safety-critical one. Other registers are exposed only as
// raw MB bytes for now — the per-register sub-decoders land as
// downstream consumers actually need them.

// CommBAltitudeReply is the decoded form of a DF 20 frame:
// the surveillance-altitude header (FS / DR / UM / altitude)
// plus a Comm-B MB payload.
type CommBAltitudeReply struct {
	FlightStatus    FlightStatus
	DownlinkRequest uint8
	UtilityMessage  uint8
	AltitudeFeet    int
	AltitudeError   error
	MB              [7]byte
	ICAO            ICAO
}

// CommBIdentityReply is the decoded form of a DF 21 frame: the
// surveillance-identity header (FS / DR / UM / squawk) plus a
// Comm-B MB payload.
type CommBIdentityReply struct {
	FlightStatus    FlightStatus
	DownlinkRequest uint8
	UtilityMessage  uint8
	Squawk          Squawk
	MB              [7]byte
	ICAO            ICAO
}

// DecodeCommBAltitude parses a DF 20 frame. icao is the addressed
// aircraft's ICAO recovered from the parity residual.
func DecodeCommBAltitude(frame Frame, icao ICAO) (CommBAltitudeReply, error) {
	if got := frame.DF(); got != DFCommBAltitude {
		return CommBAltitudeReply{}, fmt.Errorf("%w: have DF %d, want %d",
			ErrWrongDF, got, DFCommBAltitude)
	}

	if len(frame) != LongFrameBytes {
		return CommBAltitudeReply{}, fmt.Errorf("%w: have %d bytes, want %d for DF 20",
			ErrFrameTooShort, len(frame), LongFrameBytes)
	}

	header := extractSurveillanceHeader(frame)
	altitude, err := AltitudeFeet(header.payload13)

	const mbOffset = 4

	var commBPayload [7]byte
	copy(commBPayload[:], frame[mbOffset:mbOffset+7])

	return CommBAltitudeReply{
		FlightStatus:    header.flightStatus,
		DownlinkRequest: header.downlinkRequest,
		UtilityMessage:  header.utilityMessage,
		AltitudeFeet:    altitude,
		AltitudeError:   err,
		MB:              commBPayload,
		ICAO:            icao,
	}, nil
}

// DecodeCommBIdentity parses a DF 21 frame.
func DecodeCommBIdentity(frame Frame, icao ICAO) (CommBIdentityReply, error) {
	if got := frame.DF(); got != DFCommBIdentity {
		return CommBIdentityReply{}, fmt.Errorf("%w: have DF %d, want %d",
			ErrWrongDF, got, DFCommBIdentity)
	}

	if len(frame) != LongFrameBytes {
		return CommBIdentityReply{}, fmt.Errorf("%w: have %d bytes, want %d for DF 21",
			ErrFrameTooShort, len(frame), LongFrameBytes)
	}

	header := extractSurveillanceHeader(frame)

	const mbOffset = 4

	var commBPayload [7]byte
	copy(commBPayload[:], frame[mbOffset:mbOffset+7])

	return CommBIdentityReply{
		FlightStatus:    header.flightStatus,
		DownlinkRequest: header.downlinkRequest,
		UtilityMessage:  header.utilityMessage,
		Squawk:          SquawkFromIdentityCode(header.payload13),
		MB:              commBPayload,
		ICAO:            icao,
	}, nil
}

// DecodeBDS20Callsign parses a 7-byte Comm-B MB payload as a
// BDS 2,0 Aircraft Identification register. Wire layout:
//
//	bit  0..7  BDS code = 0x20 (the high nibble 2 / low nibble 0)
//	bit  8..55 8 × 6-bit characters of callsign (Mode S 6-bit alphabet)
//
// Returns the decoded callsign with trailing spaces trimmed. If
// the BDS-code byte is not 0x20 the function still attempts the
// decode but flags it via the ok return — callers can decide
// whether to trust the result.
//
//nolint:nonamedreturns // (callsign, ok) reads clearer named at this signature.
func DecodeBDS20Callsign(commBPayload [7]byte) (callsign string, ok bool) {
	const expectedCode = 0x20

	const (
		callsignChars = 8
		bitsPerChar   = 6
	)

	codeOK := commBPayload[0] == expectedCode

	const meBits = 56

	var meWord uint64
	for index := range commBPayload {
		meWord = (meWord << 8) | uint64(commBPayload[index]) //nolint:mnd // 8 bits per byte.
	}

	const charMask uint64 = 0x3F

	chars := make([]byte, 0, callsignChars)
	for charIndex := range callsignChars {
		shift := uint(meBits - 8 - (charIndex+1)*bitsPerChar) //nolint:mnd // 8-bit BDS-code prefix.
		value := byte((meWord >> shift) & charMask)
		chars = append(chars, modesAlphabet[value])
	}

	return trimTrailingSpaces(string(chars)), codeOK
}
