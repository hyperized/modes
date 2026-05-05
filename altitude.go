package modes

import "errors"

// Mode S 13-bit altitude code
// ===========================
//
// ICAO Annex 10 Vol IV §3.1.2.6.5.4. The 13-bit Altitude Code (AC)
// field appears in DF 0, DF 4, DF 16, DF 20, and the airborne-
// position ME messages of DF 17/18. Three encodings live in the
// same 13-bit slot, distinguished by the M-bit and Q-bit:
//
//	M Q  | meaning
//	-----+--------------------------------------------------
//	0 1  | feet, 25-ft increments, plain binary (modern path)
//	0 0  | feet, 100-ft increments, Gillham/Gray-coded (legacy)
//	1 -  | metres encoding (rarely seen; reserved range)
//
// Bit layout in the 13-bit field (MSB → LSB), with bit positions
// counted MSB-first from 12 down to 0:
//
//	bit 12 11 10  9  8  7  6  5  4  3  2  1  0
//	    C1 A1 C2 A2 C4 A4  M B1  Q B2 D2 B4 D4

// ErrAltitudeMSet is the static sentinel for the metric-altitude
// path (M = 1) — uncommon enough that we surface it as an error
// rather than guess at a value the spec leaves "to be defined"
// for the international agreement that has yet to land.
var ErrAltitudeMSet = errors.New("modes: metric altitude (M=1) not supported")

// ErrGillhamUnsupported is the placeholder for the legacy
// Q=0 Gillham/Gray-coded altitude path. Returned until the
// follow-up commit that lands the lookup machinery. Receivers in
// transponder-equipped airspace see Q=1 frames almost
// exclusively, so the binary path covers the vast majority of
// altitude reports.
var ErrGillhamUnsupported = errors.New("modes: Gillham (Q=0) altitude decoding not yet implemented")

// AltitudeFeet decodes a 13-bit Mode S Altitude Code into feet.
// The input must be a uint16 with only bits 12..0 set; higher
// bits are ignored.
//
//	M=0, Q=1 → modern 25-ft binary form (the common case);
//	M=0, Q=0 → legacy Gillham (100-ft Gray code) — returns
//	           ErrGillhamUnsupported until the follow-up lands;
//	M=1      → metric form, returns ErrAltitudeMSet.
func AltitudeFeet(altitudeCode uint16) (int, error) {
	const (
		mBitPos uint = 6
		qBitPos uint = 4

		mBitMask uint16 = 1 << mBitPos
		qBitMask uint16 = 1 << qBitPos

		altitudeMultiplier = 25
		altitudeOffset     = -1000
	)

	if altitudeCode&mBitMask != 0 {
		return 0, ErrAltitudeMSet
	}

	if altitudeCode&qBitMask == 0 {
		return decodeGillhamAltitude(altitudeCode)
	}

	// Strip M (bit 6) and Q (bit 4); recombine the remaining 11
	// bits MSB-first into N. The C1..A4 group (bits 12..7) takes
	// the top 6 N bits; B1 (bit 5) takes the next; B2..D4
	// (bits 3..0) take the bottom 4.
	const (
		topGroupShift   uint = 7    // bits 12..7
		topGroupNShift  uint = 5    // → N bits 10..5
		topGroupMask    uint = 0x3F // 6-bit
		middleBitNShift uint = 4    // B1 → N bit 4
		bottomMask      uint = 0x0F // bits 3..0
	)

	value := uint(altitudeCode>>topGroupShift)&topGroupMask<<topGroupNShift |
		uint(altitudeCode>>5)&1<<middleBitNShift | //nolint:mnd // B1 sits at bit 5; one-bit extract.
		uint(altitudeCode)&bottomMask

	return int(value)*altitudeMultiplier + altitudeOffset, nil
}

// decodeGillhamAltitude is the legacy 100-ft Gray-coded path.
// Returns ErrGillhamUnsupported until the follow-up
// implementation lands.
func decodeGillhamAltitude(_ uint16) (int, error) {
	return 0, ErrGillhamUnsupported
}
