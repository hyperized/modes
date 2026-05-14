package modes

import (
	"errors"
	"fmt"
)

// Extended Squitter (DF 17, DF 18)
// ===============================
//
// ICAO Annex 10 Vol IV §3.1.2.8.6 (DF 17 ES) and §3.1.2.8.7
// (DF 18 non-transponder ES). Both formats share a 14-byte
// (112-bit) frame layout:
//
//	bit 0..4    DF (5 bits)
//	bit 5..7    CA (DF 17) / CF (DF 18) — 3 bits
//	bit 8..31   AA (24 bits, broadcasting aircraft's ICAO)
//	bit 32..87  ME (56 bits, message field)
//	bit 88..111 PI (24 bits, plain CRC parity)
//
// ME's first 5 bits are the Type Code (TC) which dispatches to
// the per-message decoder; ranges per §3.1.2.8.6.4 / DO-260B
// table 2-2:
//
//	TC  1..4   Aircraft Identification (callsign + category)
//	TC  5..8   Surface Position (CPR-encoded latitude/longitude)
//	TC  9..18  Airborne Position (barometric altitude)
//	TC 19      Airborne Velocity
//	TC 20..22  Airborne Position (GNSS altitude)
//	TC 23      Test Message
//	TC 24..27  Reserved (TIS-B / ADS-R rebroadcast on DF 18)
//	TC 28      Aircraft Status (emergency state, ACAS RA)
//	TC 29      Target State and Status Information
//	TC 30      Aircraft Operational Coordination
//	TC 31      Aircraft Operational Status

// TypeCode is the 5-bit field at the head of the ME payload.
type TypeCode uint8

// ExtendedSquitter is the structural decoded form of a DF 17 or
// DF 18 frame: the broadcasting aircraft's ICAO, the 3-bit CA/CF
// header, and the typed ME message. The Message field is one of
// the per-TC types — type-switch to extract.
type ExtendedSquitter struct {
	DF       DownlinkFormat
	Category uint8 // CA for DF 17, CF for DF 18
	ICAO     ICAO
	TypeCode TypeCode
	Message  Message
}

// Message is the sealed interface every TC-specific decoded
// payload satisfies. The concrete types (Identification,
// AirbornePosition, AirborneVelocity, …) land alongside their
// per-TC decoders.
type Message interface {
	isModesMessage()
}

// ErrUnsupportedTypeCode is returned by DecodeExtendedSquitter
// when the ME field carries a Type Code we don't have a decoder
// for yet. The caller still receives the structural ExtendedSquitter
// (DF / CA / AA / TC populated) so it can log or count the
// unhandled type without losing the broadcasting aircraft's
// identity.
var ErrUnsupportedTypeCode = errors.New("modes: type code not yet supported")

// DecodeExtendedSquitter parses a DF 17 or DF 18 frame and
// dispatches the ME payload to the per-Type-Code decoder. The
// structural fields (DF, Category, ICAO, TypeCode) are always
// populated; Message is non-nil iff the TC has a registered
// decoder.
//
// Returns ErrWrongDF for non-DF-17/18 frames, ErrFrameTooShort
// for frames shorter than LongFrameBytes, and any per-TC decoder
// error wrapped with ErrUnsupportedTypeCode for TCs without a
// registered decoder yet.
func DecodeExtendedSquitter(frame Frame) (ExtendedSquitter, error) {
	// Length-check first: Frame.DF() panics on an empty slice, so
	// surfacing ErrFrameTooShort for the empty case keeps the
	// public Decode* surface panic-free for arbitrary input.
	if len(frame) != LongFrameBytes {
		return ExtendedSquitter{}, fmt.Errorf("%w: have %d bytes, want %d for ES",
			ErrFrameTooShort, len(frame), LongFrameBytes)
	}

	got := frame.DF()
	if got != DFExtendedSquitter && got != DFNonTransponderES {
		return ExtendedSquitter{}, fmt.Errorf("%w: have DF %d, want 17 or 18", ErrWrongDF, got)
	}

	const (
		categoryMask  byte = 0x07
		typeCodeShift byte = 3

		aaOffset = 1
		meOffset = 4
		meLength = 7
	)

	mePayload := frame[meOffset : meOffset+meLength]
	typeCode := TypeCode(mePayload[0] >> typeCodeShift)

	squitter := ExtendedSquitter{
		DF:       got,
		Category: frame[0] & categoryMask,
		ICAO:     extractICAO24(frame, aaOffset),
		TypeCode: typeCode,
	}

	message, err := decodeMEPayload(typeCode, mePayload)
	if err != nil {
		// Return the structural data so callers can still see
		// the broadcasting ICAO + TC even when we don't yet have
		// a payload decoder.
		return squitter, err
	}

	squitter.Message = message

	return squitter, nil
}

// decodeMEPayload dispatches the 7-byte ME slice to a per-TC
// decoder. New Type Codes land by extending the conditional
// chain as their decoders ship.
//
//nolint:ireturn // dispatcher returns the per-TC concrete via the sealed Message interface.
func decodeMEPayload(typeCode TypeCode, mePayload []byte) (Message, error) {
	if typeCode >= 1 && typeCode <= 4 { //nolint:mnd // TC range from spec table 2-2.
		return decodeIdentification(typeCode, mePayload), nil
	}

	if typeCode >= 5 && typeCode <= 8 { //nolint:mnd // TC range = surface position.
		return decodeSurfacePosition(typeCode, mePayload), nil
	}

	if (typeCode >= 9 && typeCode <= 18) || (typeCode >= 20 && typeCode <= 22) { //nolint:mnd // TC ranges per spec.
		return decodeAirbornePosition(typeCode, mePayload), nil
	}

	if typeCode == 19 { //nolint:mnd // TC 19 = Airborne Velocity.
		return decodeVelocity(mePayload), nil
	}

	if typeCode == 28 { //nolint:mnd // TC 28 = Aircraft Status (emergency / TCAS RA).
		return decodeAircraftStatus(mePayload), nil
	}

	if typeCode == 29 { //nolint:mnd // TC 29 = Target State and Status.
		return decodeTargetState(mePayload), nil
	}

	if typeCode == 31 { //nolint:mnd // TC 31 = Aircraft Operational Status.
		return decodeOperationalStatus(mePayload), nil
	}

	return nil, fmt.Errorf("%w: TC %d", ErrUnsupportedTypeCode, typeCode)
}
