package modes

// Mode 3/A squawk decoding (13-bit ID field)
// ==========================================
//
// ICAO Annex 10 Vol IV §3.1.2.6.7. The 13-bit ID field appears in
// DF 5 (surveillance identity reply) and the corresponding Comm-B
// identity reply (DF 21). It carries the four-digit octal "squawk"
// the pilot has dialled into the transponder — for example 1200
// (US VFR), 7700 (emergency), 7600 (radio failure), 7500 (hijack).
//
// Bit layout (MSB → LSB), bit position counting from 12 down to 0:
//
//	bit 12 11 10  9  8  7  6  5  4  3  2  1  0
//	    C1 A1 C2 A2 C4 A4  X B1 D1 B2 D2 B4 D4
//
// Each octal digit comes from three bits: A is (A4 A2 A1), B is
// (B4 B2 B1), C is (C4 C2 C1), D is (D4 D2 D1). The bits are
// plain binary — no Gray code — so each three-bit group is
// directly the decimal value of the corresponding octal digit
// (0..7).
//
// X is the always-zero "spare" bit at position 6 (ICAO reserved;
// transponders set it to 0). We do not enforce X = 0 because some
// implementations have been observed to set it spuriously, and
// the spec does not make X a parity bit.

// Squawk is a 4-digit octal Mode 3/A identity code. Stored as a
// uint16 holding the literal four-digit octal as decimal —
// e.g. squawk "7700" is the value 7700, not 0o7700 = 4032 — so
// formatting it for display is `fmt.Sprintf("%04d", squawk)`.
type Squawk uint16

// SquawkFromIdentityCode unpacks the 13-bit ID field into a
// four-octal-digit Squawk. Input bits above position 12 are
// ignored. The function never returns an error: every 13-bit
// pattern is a syntactically valid (though not necessarily
// pilot-meaningful) squawk code.
func SquawkFromIdentityCode(identityCode uint16) Squawk {
	const (
		// Each octal digit's three bits live at the documented
		// positions in the ID field.
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

		// Each digit's place value when assembling the four
		// digits into a single decimal-of-the-octal-string
		// integer (e.g. squawk "7700" = 7000+700+00+0).
		placeA = 1000
		placeB = 100
		placeC = 10
		placeD = 1
	)

	digit := func(bit1, bit2, bit4 uint16) int {
		var value int
		if identityCode&bit1 != 0 {
			value |= 1
		}

		if identityCode&bit2 != 0 {
			value |= 2 //nolint:mnd // octal-digit bit weight.
		}

		if identityCode&bit4 != 0 {
			value |= 4 //nolint:mnd // octal-digit bit weight.
		}

		return value
	}

	a := digit(bitA1, bitA2, bitA4)
	b := digit(bitB1, bitB2, bitB4)
	c := digit(bitC1, bitC2, bitC4)
	d := digit(bitD1, bitD2, bitD4)

	return Squawk(a*placeA + b*placeB + c*placeC + d*placeD) //nolint:gosec // value bounded by 7777.
}
