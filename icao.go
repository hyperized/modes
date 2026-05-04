package modes

// ICAO is a 24-bit ICAO aircraft address. Every Mode S transponder
// is permanently assigned one (issued via the State of Registry
// per ICAO Annex 10 Vol III §9.8.2). Even though the wire encoding
// is 24 bits, ICAO addresses are conventionally written and stored
// as 32-bit unsigned integers with the top byte zero — Go has no
// native uint24, and the ergonomics of uint32 outweigh the eight
// wasted bits.
//
// Zero is reserved (no aircraft is allocated address 0); a zero
// ICAO is a sentinel for "no address" or "all-call broadcast".
type ICAO uint32

// extractICAO24 reads three big-endian bytes from data starting at
// offset and packs them into the low 24 bits of an ICAO value.
// data must hold at least offset+3 bytes; callers ensure that via
// the per-DF decoder's frame-length check.
func extractICAO24(data []byte, offset int) ICAO {
	const (
		highShift = 16
		midShift  = 8
	)

	return ICAO(data[offset])<<highShift |
		ICAO(data[offset+1])<<midShift |
		ICAO(data[offset+2])
}
