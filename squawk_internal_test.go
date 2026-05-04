package modes

import "testing"

// makeIdentityCode packs an octal-as-decimal squawk (e.g. 7700)
// into a 13-bit ID field. Used by round-trip tests to verify the
// decoder against synthetic but bit-exact inputs.
func makeIdentityCode(squawk Squawk) uint16 {
	const (
		placeA = 1000
		placeB = 100
		placeC = 10
		placeD = 1

		bitA1 uint16 = 1 << 11
		bitA2 uint16 = 1 << 9
		bitA4 uint16 = 1 << 7

		bitB1 uint16 = 1 << 5
		bitB2 uint16 = 1 << 3
		bitB4 uint16 = 1 << 1

		bitC1 uint16 = 1 << 12
		bitC2 uint16 = 1 << 10
		bitC4 uint16 = 1 << 8

		bitD1 uint16 = 1 << 4
		bitD2 uint16 = 1 << 2
		bitD4 uint16 = 1 << 0
	)

	digitA := int(squawk) / placeA % 10
	digitB := int(squawk) / placeB % 10
	digitC := int(squawk) / placeC % 10
	digitD := int(squawk) / placeD % 10

	pack := func(value int, bit1, bit2, bit4 uint16) uint16 {
		var out uint16

		if value&1 != 0 {
			out |= bit1
		}

		//nolint:mnd // octal-digit bit weights.
		if value&2 != 0 {
			out |= bit2
		}

		//nolint:mnd // octal-digit bit weights.
		if value&4 != 0 {
			out |= bit4
		}

		return out
	}

	return pack(digitA, bitA1, bitA2, bitA4) |
		pack(digitB, bitB1, bitB2, bitB4) |
		pack(digitC, bitC1, bitC2, bitC4) |
		pack(digitD, bitD1, bitD2, bitD4)
}

func TestSquawkFromIdentityCodeKnownCodes(t *testing.T) {
	t.Parallel()

	// Well-known squawks — pin a few so a future bit-position
	// typo breaks loudly with a recognisable failure.
	for _, want := range []Squawk{
		0,    // factory default
		1200, // US VFR
		2000, // ICAO standard "uncontrolled" / IFR
		7000, // EU VFR
		7500, // hijack
		7600, // radio failure
		7700, // emergency
		7777, // all bits set
	} {
		got := SquawkFromIdentityCode(makeIdentityCode(want))
		if got != want {
			t.Errorf("SquawkFromIdentityCode(makeIdentityCode(%04d)) = %04d, want %04d", want, got, want)
		}
	}
}
