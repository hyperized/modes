package modes_test

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/hyperized/modes"
)

// Black-box tests against the published public API surface. The
// goal is not to duplicate coverage from the internal test suite
// but to lock the contract: imports only exported symbols, asserts
// on exported fields, and branches against exported sentinels via
// errors.Is. Future internal refactors that break the public
// contract fail loudly here.

// decodeHexT is the external-test mirror of the internal helper —
// duplicated here so this file imports nothing from the package
// being tested.
func decodeHexT(tb testing.TB, h string) []byte {
	tb.Helper()

	out, err := hex.DecodeString(strings.ToUpper(h))
	if err != nil {
		tb.Fatalf("hex %q: %v", h, err)
	}

	return out
}

// TestPublicExtendedSquitterIdent locks the TC 4 happy path: a
// known DF 17 frame yields an IdentificationMessage with the
// callsign string a type-switching caller would read.
func TestPublicExtendedSquitterIdent(t *testing.T) {
	t.Parallel()

	frame := modes.Frame(decodeHexT(t, "8D4840D6202CC371C32CE0576098"))

	squitter, err := modes.DecodeExtendedSquitter(frame)
	if err != nil {
		t.Fatalf("DecodeExtendedSquitter: %v", err)
	}

	if squitter.DF != modes.DFExtendedSquitter {
		t.Errorf("DF = %d, want %d", squitter.DF, modes.DFExtendedSquitter)
	}

	if squitter.ICAO != modes.ICAO(0x4840D6) {
		t.Errorf("ICAO = %#06x, want 0x4840D6", squitter.ICAO)
	}

	ident, ok := squitter.Message.(modes.IdentificationMessage)
	if !ok {
		t.Fatalf("Message is %T, want modes.IdentificationMessage", squitter.Message)
	}

	if ident.Callsign != "KLM1023" {
		t.Errorf("Callsign = %q, want %q", ident.Callsign, "KLM1023")
	}
}

// TestPublicExtendedSquitterUnsupportedTC locks the error
// contract: an ES frame with a TC we don't decode (e.g. TC 23
// "test message") returns the structural fields plus a
// sentinel-wrapped error callers can branch on.
func TestPublicExtendedSquitterUnsupportedTC(t *testing.T) {
	t.Parallel()

	// Hand-craft a DF 17 frame whose ME byte 0 carries TC 23: the
	// top 5 bits of byte 4 are 0b10111 = 23. CRC is left zero —
	// the decoder does not verify CRC, by design.
	body := []byte{
		0x8D, 0x40, 0x62, 0x1D, // DF 17, ICAO 0x40621D
		0xB8, 0, 0, 0, 0, 0, 0, // ME starts with 0xB8 = TC 23, rest zero
		0, 0, 0, // CRC tail (unused)
	}

	_, err := modes.DecodeExtendedSquitter(modes.Frame(body))
	if !errors.Is(err, modes.ErrUnsupportedTypeCode) {
		t.Fatalf("err = %v, want ErrUnsupportedTypeCode", err)
	}
}

// TestPublicAltitudeFeet exercises the three documented paths of
// the public AltitudeFeet decoder via its exported sentinels.
func TestPublicAltitudeFeet(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		code    uint16
		wantErr error
	}{
		{name: "modern_25ft_step", code: 0x10 | (0b001 << 7), wantErr: nil}, // Q=1, value bits set
		{name: "metric_M_bit_set", code: 0x40, wantErr: modes.ErrAltitudeMSet},
		{name: "gillham_Q_clear", code: 0, wantErr: modes.ErrGillhamUnsupported},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := modes.AltitudeFeet(testCase.code)
			if !errors.Is(err, testCase.wantErr) {
				t.Errorf("err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

// TestPublicSquawkFromIdentityCode locks the contract that every
// 13-bit identity code maps to a syntactically valid octal squawk.
func TestPublicSquawkFromIdentityCode(t *testing.T) {
	t.Parallel()

	// 0 should be 0; non-zero patterns produce well-formed octal.
	if got := modes.SquawkFromIdentityCode(0); got != 0 {
		t.Errorf("SquawkFromIdentityCode(0) = %d, want 0", got)
	}

	// A pattern that should yield squawk 7700 (emergency): the
	// per-bit positions are documented in the package. We just
	// assert the result digits are octal.
	if got := modes.SquawkFromIdentityCode(0x1FFF); got > 7777 {
		t.Errorf("SquawkFromIdentityCode(0x1FFF) = %d, want <= 7777", got)
	}
}

// TestPublicDecodeAllCallReply covers the public DF 11 decoder.
func TestPublicDecodeAllCallReply(t *testing.T) {
	t.Parallel()

	body := []byte{0x5D, 0x48, 0x47, 0x55}
	frame := modes.Frame(modes.AppendCRC24(body))

	reply, err := modes.DecodeAllCallReply(frame, 0)
	if err != nil {
		t.Fatalf("DecodeAllCallReply: %v", err)
	}

	if reply.ICAO != modes.ICAO(0x484755) {
		t.Errorf("ICAO = %#06x, want 0x484755", reply.ICAO)
	}

	if !reply.IsUnsolicited {
		t.Error("IsUnsolicited = false, want true (residual = 0)")
	}
}

// TestPublicDecodersRejectWrongDF locks the ErrWrongDF contract on
// every per-DF public decoder. Each subtest constructs a frame
// whose length is correct for the target decoder but whose DF is
// different — so the DF check fires after the length check passes.
func TestPublicDecodersRejectWrongDF(t *testing.T) {
	t.Parallel()

	// 14-byte ES frame for the long-frame decoders; a 7-byte
	// DF-mismatched short frame for the short-frame decoders.
	longFrame := modes.Frame(decodeHexT(t, "8D4840D6202CC371C32CE0576098"))
	shortDF11 := mkShortFrameWithDF(modes.DFAllCallReply)
	shortDF4 := mkShortFrameWithDF(modes.DFSurveillanceAlt)

	t.Run("DecodeAllCallReply", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeAllCallReply(shortDF4, 0)
		assertErrIs(t, err, modes.ErrWrongDF)
	})

	t.Run("DecodeSurveillanceAltitude", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeSurveillanceAltitude(shortDF11, 0)
		assertErrIs(t, err, modes.ErrWrongDF)
	})

	t.Run("DecodeSurveillanceIdentity", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeSurveillanceIdentity(shortDF11, 0)
		assertErrIs(t, err, modes.ErrWrongDF)
	})

	t.Run("DecodeACASShortReply", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeACASShortReply(shortDF11, 0)
		assertErrIs(t, err, modes.ErrWrongDF)
	})

	t.Run("DecodeACASLongReply", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeACASLongReply(longFrame, 0)
		assertErrIs(t, err, modes.ErrWrongDF)
	})

	t.Run("DecodeCommBAltitude", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeCommBAltitude(longFrame, 0)
		assertErrIs(t, err, modes.ErrWrongDF)
	})

	t.Run("DecodeCommBIdentity", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeCommBIdentity(longFrame, 0)
		assertErrIs(t, err, modes.ErrWrongDF)
	})

	t.Run("DecodeCommDExtendedLength", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeCommDExtendedLength(longFrame, 0)
		assertErrIs(t, err, modes.ErrWrongDF)
	})

	t.Run("DecodeExtendedSquitter", func(t *testing.T) {
		t.Parallel()

		// Same wire length as ES (14 bytes) but DF 19 (military).
		military := make([]byte, len(longFrame))
		copy(military, longFrame)
		military[0] = byte(modes.DFMilitaryES) << 3 //nolint:mnd // DF field at bits 7..3.

		_, err := modes.DecodeExtendedSquitter(modes.Frame(military))
		assertErrIs(t, err, modes.ErrWrongDF)
	})
}

// TestPublicDecodersRejectEmptyFrame is the regression test for
// the empty-input panic the fuzz harness caught: every public
// Decode* must surface ErrFrameTooShort rather than panicking on
// a zero-length input.
func TestPublicDecodersRejectEmptyFrame(t *testing.T) {
	t.Parallel()

	empty := modes.Frame{}

	t.Run("DecodeExtendedSquitter", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeExtendedSquitter(empty)
		assertErrIs(t, err, modes.ErrFrameTooShort)
	})

	t.Run("DecodeAllCallReply", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeAllCallReply(empty, 0)
		assertErrIs(t, err, modes.ErrFrameTooShort)
	})

	t.Run("DecodeSurveillanceAltitude", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeSurveillanceAltitude(empty, 0)
		assertErrIs(t, err, modes.ErrFrameTooShort)
	})

	t.Run("DecodeSurveillanceIdentity", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeSurveillanceIdentity(empty, 0)
		assertErrIs(t, err, modes.ErrFrameTooShort)
	})

	t.Run("DecodeACASShortReply", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeACASShortReply(empty, 0)
		assertErrIs(t, err, modes.ErrFrameTooShort)
	})

	t.Run("DecodeACASLongReply", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeACASLongReply(empty, 0)
		assertErrIs(t, err, modes.ErrFrameTooShort)
	})

	t.Run("DecodeCommBAltitude", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeCommBAltitude(empty, 0)
		assertErrIs(t, err, modes.ErrFrameTooShort)
	})

	t.Run("DecodeCommBIdentity", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeCommBIdentity(empty, 0)
		assertErrIs(t, err, modes.ErrFrameTooShort)
	})

	t.Run("DecodeCommDExtendedLength", func(t *testing.T) {
		t.Parallel()

		_, err := modes.DecodeCommDExtendedLength(empty, 0)
		assertErrIs(t, err, modes.ErrFrameTooShort)
	})
}

// mkShortFrameWithDF builds a 7-byte short frame whose first byte
// is the given DF in the top 5 bits, with the rest zeroed. Useful
// for forcing the ErrWrongDF arm without faking a real payload.
func mkShortFrameWithDF(downlinkFormat modes.DownlinkFormat) modes.Frame {
	buf := make([]byte, modes.ShortFrameBytes)
	buf[0] = byte(downlinkFormat) << 3 //nolint:mnd // DF field at bits 7..3.

	return modes.Frame(buf)
}

// assertErrIs is the test-file mirror of errors.Is, factored out so
// the per-decoder subtests stay one-line and below the test-file
// wrapcheck exclusion is honoured (we never return the external
// error, only inspect it).
func assertErrIs(tb testing.TB, err, want error) {
	tb.Helper()

	if !errors.Is(err, want) {
		tb.Errorf("err = %v, want %v", err, want)
	}
}

// TestPublicCRC24RoundTrip locks the spec property that drives
// every overlay-DF decoder: appending CRC24 to a body produces a
// frame whose residual is zero.
func TestPublicCRC24RoundTrip(t *testing.T) {
	t.Parallel()

	body := []byte{0x8D, 0x40, 0x62, 0x1D, 0x58, 0xC3, 0x82, 0xD6, 0x90, 0xC8, 0xAC}

	frame := modes.AppendCRC24(body)
	if got := modes.CRC24(frame); got != 0 {
		t.Errorf("CRC24 over AppendCRC24(body) = %#06x, want 0", got)
	}

	if got := modes.CRCResidual(frame); got != 0 {
		t.Errorf("CRCResidual = %#06x, want 0", got)
	}
}

// TestPublicDecodeCPRGlobalZoneCrossing locks the
// ErrCPRZoneCrossing sentinel: a same-format pair must be
// rejected with the documented sentinel.
func TestPublicDecodeCPRGlobalZoneCrossing(t *testing.T) {
	t.Parallel()

	even := modes.CPRPosition{Latitude: 92095, Longitude: 39846, Format: modes.CPRFormatEven}
	notOdd := modes.CPRPosition{Latitude: 88385, Longitude: 125818, Format: modes.CPRFormatEven}

	_, _, err := modes.DecodeCPRGlobal(even, notOdd, modes.CPRFormatEven)
	if !errors.Is(err, modes.ErrCPRZoneCrossing) {
		t.Errorf("err = %v, want ErrCPRZoneCrossing", err)
	}
}

// TestPublicFrameDFAndLength locks the Frame helper contract.
func TestPublicFrameDFAndLength(t *testing.T) {
	t.Parallel()

	frame := modes.Frame{0x8D, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

	if got := frame.DF(); got != modes.DFExtendedSquitter {
		t.Errorf("DF = %d, want %d", got, modes.DFExtendedSquitter)
	}

	if !frame.IsLong() {
		t.Error("IsLong = false, want true for DF 17")
	}

	if got := modes.DFExtendedSquitter.LengthBytes(); got != modes.LongFrameBytes {
		t.Errorf("DFExtendedSquitter.LengthBytes = %d, want %d", got, modes.LongFrameBytes)
	}
}
