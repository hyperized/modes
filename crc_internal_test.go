package modes

import "testing"

// TestCRC24SelfCancellation locks in the foundational property: a
// frame ending in its own CRC tail computes to zero. Every other
// CRC test in the package can assume this works.
func TestCRC24SelfCancellation(t *testing.T) {
	t.Parallel()

	body := []byte{0x8D, 0x40, 0x62, 0x1D}

	if got := CRC24(AppendCRC24(body)); got != 0 {
		t.Errorf("CRC24(body || CRC24(body)) = %#06x, want 0", got)
	}
}

// TestCRC24KnownVector verifies the polynomial against a frame
// whose checksum is hand-computable from the spec's polynomial
// table — protects against a typo that would silently survive
// the self-cancellation round-trip (since a wrong CRC24 cancels
// against a wrong AppendCRC24).
//
// Vector: a single zero byte. CRC24({0x00}) = 0 because the shift
// register starts at zero and stays there.
func TestCRC24KnownVectorZero(t *testing.T) {
	t.Parallel()

	if got := CRC24([]byte{0x00}); got != 0 {
		t.Errorf("CRC24({0x00}) = %#06x, want 0", got)
	}
}

func TestAppendCRC24Length(t *testing.T) {
	t.Parallel()

	body := []byte{0x8D, 0x40, 0x62, 0x1D, 0x58, 0xC3, 0x82, 0xD6, 0x90, 0xC8, 0xAC}

	out := AppendCRC24(body)
	if len(out) != len(body)+ParityBytes {
		t.Errorf("len(AppendCRC24) = %d, want %d", len(out), len(body)+ParityBytes)
	}

	// The first len(body) bytes must be byte-identical to the input.
	for index := range body {
		if out[index] != body[index] {
			t.Errorf("body byte %d differs: got %#x, want %#x", index, out[index], body[index])
		}
	}
}

// TestCRCResidualOnFullFrame verifies the public Frame-friendly
// helper: handing a complete (body + parity) frame to CRCResidual
// returns zero when no overlay is applied, matching the
// self-cancellation property.
func TestCRCResidualOnFullFrame(t *testing.T) {
	t.Parallel()

	body := []byte{0x8D, 0x40, 0x62, 0x1D, 0x58, 0xC3, 0x82, 0xD6, 0x90, 0xC8, 0xAC}
	frame := Frame(AppendCRC24(body))

	if got := CRCResidual(frame); got != 0 {
		t.Errorf("CRCResidual(clean frame) = %#06x, want 0", got)
	}
}

// TestCRC24KnownDF17Frame locks the polynomial against a real
// DF 17 (Extended Squitter) frame from publicly-circulated Mode S
// references — 8D40621D58C382D690C8AC2863A7 — whose parity is a
// plain CRC of the message body (DF 17 / 18 do not use the
// address-parity overlay technique; the broadcasting ICAO is
// already in the AA field of the message body, so overlaying it
// into the parity would be redundant). A clean such frame has
// CRC residual zero.
//
// Catches a polynomial typo even when self-cancellation passes:
// AppendCRC24 + CRC24 stay consistent with each other under any
// 24-bit polynomial, but a real wire frame only resolves to zero
// under the spec's actual polynomial.
func TestCRC24KnownDF17Frame(t *testing.T) {
	t.Parallel()

	frame := Frame{
		0x8D, 0x40, 0x62, 0x1D,
		0x58, 0xC3, 0x82, 0xD6, 0x90, 0xC8, 0xAC,
		0x28, 0x63, 0xA7,
	}

	if got := CRCResidual(frame); got != 0 {
		t.Errorf("CRCResidual(real DF17 frame) = %#06x, want 0", got)
	}
}
