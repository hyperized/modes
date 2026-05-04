package modes

import (
	"errors"
	"testing"
)

// makeESFrame synthesises a DF 17 (or DF 18) frame with the given
// CA, ICAO, and 7-byte ME payload. The PI tail is filled by
// AppendCRC24 (DF 17/18 use plain CRC parity, so the synthesised
// frame self-cancels — matching the wire reality).
func makeESFrame(downlinkFormat DownlinkFormat, category uint8, icao ICAO, mePayload [7]byte) Frame {
	const (
		dfShift   = 3
		highShift = 16
		midShift  = 8
		byteMask  = 0xff
	)

	body := make([]byte, 0, 4+len(mePayload))
	body = append(body,
		byte(downlinkFormat<<dfShift)|(category&0x07),
		byte((icao>>highShift)&byteMask),
		byte((icao>>midShift)&byteMask),
		byte(icao&byteMask),
	)
	body = append(body, mePayload[:]...)

	return Frame(AppendCRC24(body))
}

// pack6BitCallsign packs an 8-character callsign string (padded
// with spaces if shorter) into the 48-bit callsign field of a
// TC 1..4 ME payload. Returns the 7-byte ME payload with the
// supplied TC + category in the first byte's bottom bits.
func pack6BitCallsign(typeCode TypeCode, category uint8, callsign string) [7]byte {
	const (
		callsignChars  = 8
		bitsPerChar    = 6
		callsignBitLen = callsignChars * bitsPerChar

		tcShift         = 3
		meBits          = 56
		charMask uint64 = 0x3F
	)

	// Pad/truncate callsign to 8 characters, uppercase + spaces.
	padded := make([]byte, callsignChars)

	for index := range padded {
		padded[index] = ' '
	}

	for index := 0; index < callsignChars && index < len(callsign); index++ {
		padded[index] = callsign[index]
	}

	// Reverse-lookup each character into its 6-bit code.
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

	var meWord uint64

	meWord |= uint64(typeCode) << (meBits - 5) //nolint:mnd // 5-bit TC.
	meWord |= uint64(category&0x07) << (meBits - 8)

	for index, char := range padded {
		shift := uint(meBits - 8 - (index+1)*bitsPerChar)
		meWord |= encode(char) << shift
	}

	var out [7]byte
	for index := range out {
		out[index] = byte((meWord >> uint(meBits-8-index*8)) & 0xFF) //nolint:mnd // big-endian byte slicing.
	}

	return out
}

func TestDecodeExtendedSquitterCallsign(t *testing.T) {
	t.Parallel()

	const (
		wantICAO     ICAO   = 0x484755
		wantCallsign string = "KLM1023"
		wantCategory uint8  = 5 // arbitrary 3-bit code in set A
	)

	mePayload := pack6BitCallsign(4, wantCategory, wantCallsign) // TC 4 → category set A
	frame := makeESFrame(DFExtendedSquitter, 0, wantICAO, mePayload)

	squitter, err := DecodeExtendedSquitter(frame)
	if err != nil {
		t.Fatalf("DecodeExtendedSquitter: %v", err)
	}

	if squitter.ICAO != wantICAO {
		t.Errorf("ICAO = %#06x, want %#06x", squitter.ICAO, wantICAO)
	}

	if squitter.TypeCode != 4 {
		t.Errorf("TypeCode = %d, want 4", squitter.TypeCode)
	}

	ident, ok := squitter.Message.(IdentificationMessage)
	if !ok {
		t.Fatalf("Message is %T, want IdentificationMessage", squitter.Message)
	}

	if ident.CategorySet != 'A' {
		t.Errorf("CategorySet = %c, want A (TC 4 → set A)", ident.CategorySet)
	}

	if ident.EmitterCategory != wantCategory {
		t.Errorf("EmitterCategory = %d, want %d", ident.EmitterCategory, wantCategory)
	}

	if ident.Callsign != wantCallsign {
		t.Errorf("Callsign = %q, want %q", ident.Callsign, wantCallsign)
	}
}

func TestDecodeExtendedSquitterDF18Accepted(t *testing.T) {
	t.Parallel()

	mePayload := pack6BitCallsign(3, 1, "TEST")
	frame := makeESFrame(DFNonTransponderES, 0, 0xABCDEF, mePayload)

	squitter, err := DecodeExtendedSquitter(frame)
	if err != nil {
		t.Fatalf("DecodeExtendedSquitter: %v", err)
	}

	ident, ok := squitter.Message.(IdentificationMessage)
	if !ok {
		t.Fatalf("Message is %T, want IdentificationMessage", squitter.Message)
	}

	if ident.CategorySet != 'B' {
		t.Errorf("CategorySet = %c, want B (TC 3 → set B)", ident.CategorySet)
	}
}

func TestDecodeExtendedSquitterUnsupportedTC(t *testing.T) {
	t.Parallel()

	// TC 23 (test message) — decoder lands later. Structural
	// fields must still be populated even when ME decoding fails.
	mePayload := [7]byte{23 << 3, 0, 0, 0, 0, 0, 0}
	frame := makeESFrame(DFExtendedSquitter, 0, 0x123456, mePayload)

	squitter, err := DecodeExtendedSquitter(frame)
	if !errors.Is(err, errUnsupportedTypeCode) {
		t.Errorf("err = %v, want errUnsupportedTypeCode", err)
	}

	if squitter.ICAO != 0x123456 {
		t.Errorf("ICAO = %#06x, want 0x123456 (structural fields must be populated even on TC error)",
			squitter.ICAO)
	}

	if squitter.TypeCode != 23 {
		t.Errorf("TypeCode = %d, want 23", squitter.TypeCode)
	}
}

func TestDecodeExtendedSquitterRejectsWrongDF(t *testing.T) {
	t.Parallel()

	frame := make(Frame, LongFrameBytes)
	frame[0] = byte(DFLongAirAir << 3)

	if _, err := DecodeExtendedSquitter(frame); !errors.Is(err, errWrongDF) {
		t.Errorf("err = %v, want errWrongDF", err)
	}
}

func TestDecodeExtendedSquitterRejectsShortFrame(t *testing.T) {
	t.Parallel()

	frame := Frame{byte(DFExtendedSquitter << 3)}
	if _, err := DecodeExtendedSquitter(frame); !errors.Is(err, errFrameTooShort) {
		t.Errorf("err = %v, want errFrameTooShort", err)
	}
}

func TestDecodeIdentificationCallsignTrimsTrailingSpaces(t *testing.T) {
	t.Parallel()

	mePayload := pack6BitCallsign(4, 0, "AB1   ")

	ident := decodeIdentification(4, mePayload[:])
	if ident.Callsign != "AB1" {
		t.Errorf("Callsign = %q, want %q (trailing spaces stripped)", ident.Callsign, "AB1")
	}
}

func TestDecodeIdentificationUndefinedCharSurfacesAsHash(t *testing.T) {
	t.Parallel()

	// Build a 7-byte ME with TC=4, category=0, and the first
	// callsign character set to value 27 (undefined).
	const (
		bitsPerChar = 6
		meBits      = 56
		undef       = 27
	)

	meWord := uint64(4) << (meBits - 5) //nolint:mnd // 5-bit TC.
	meWord |= uint64(undef) << (meBits - 8 - bitsPerChar)

	// Remaining 7 chars as spaces (32) so they decode as " ".
	for index := 1; index < 8; index++ {
		meWord |= uint64(32) << uint(meBits-8-(index+1)*bitsPerChar)
	}

	var mePayload [7]byte
	for index := range mePayload {
		mePayload[index] = byte((meWord >> uint(meBits-8-index*8)) & 0xFF)
	}

	ident := decodeIdentification(4, mePayload[:])
	if len(ident.Callsign) == 0 || ident.Callsign[0] != modesUndefinedChar {
		t.Errorf("Callsign[0] = %q, want %q", ident.Callsign, modesUndefinedChar)
	}
}
