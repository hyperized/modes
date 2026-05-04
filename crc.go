package modes

// Mode S CRC-24
// =============
//
// Every Mode S downlink frame carries a 24-bit parity field at
// its tail. The parity is *not* a plain checksum — it is XORed
// with an "address parity" (AP) value that depends on the DF:
//
//	DF  | parity overlay
//	----+-------------------------------------------------------
//	17  | broadcasting aircraft's 24-bit ICAO address
//	18  | broadcasting aircraft's 24-bit ICAO address
//	11  | interrogator ID (zero for unsolicited replies)
//	0   | addressed aircraft's 24-bit ICAO address
//	4   | addressed aircraft's 24-bit ICAO address
//	5   | addressed aircraft's 24-bit ICAO address
//	16  | addressed aircraft's 24-bit ICAO address
//	20  | addressed aircraft's 24-bit ICAO address
//	21  | addressed aircraft's 24-bit ICAO address
//	24+ | addressed aircraft's 24-bit ICAO address
//
// The receiver computes CRC over the entire frame (message +
// parity tail) and gets back the AP value. For DF 11 a clean
// frame yields zero (no interrogator queried it); for the
// ICAO-overlay DFs the receiver needs a way to recognise plausible
// 24-bit addresses (an ICAO roster, a cache of recently-seen
// addresses, or a "trust the residual" policy).
//
// Generator polynomial (ICAO Annex 10 Vol IV §3.1.2.3.3.2):
//
//	G(x) = x²⁴ + x²³ + x²² + x²¹ + x²⁰ + x¹⁹ + x¹⁸ + x¹⁷ + x¹⁶
//	     + x¹⁵ + x¹⁴ + x¹³ + x¹² + x¹⁰ + x³ + 1
//
// In 24-bit "feedback" form: 0xFFF409. The implicit x²⁴ term is
// what the bit-by-bit shift register cancels against on overflow.

const (
	// crcPoly is the 24-bit feedback form of the Mode S
	// generator polynomial. The implicit x²⁴ term is handled by
	// the per-bit shift below.
	crcPoly uint32 = 0xFFF409

	// crcMask keeps the running remainder inside 24 bits after
	// each shift; without it, overflow into bit 24 would never
	// trigger the polynomial XOR cancellation.
	crcMask uint32 = 0xFFFFFF

	// crcMSB is the top bit of the 24-bit register. When set
	// before a shift, the polynomial gets XORed in to cancel
	// the implicit top bit.
	crcMSB uint32 = 0x800000

	// crcByteShift positions a freshly-fed input byte at the top
	// of the 24-bit register so its bits propagate through the
	// per-bit loop in MSB-first order.
	crcByteShift uint32 = 16
)

// CRC24 computes the Mode S 24-bit checksum of data using the
// classic bit-by-bit shift register. For any byte sequence X the
// concatenation X || CRC24(X) has CRC24 = 0 — that's the property
// the receiver exploits to detect transmission errors.
//
// 14-byte (long) frames take 112 iterations; the bit-by-bit loop
// runs comfortably under a microsecond. A precomputed 256-entry
// table would shave more cycles but obscures the polynomial; the
// allocation-free property matters more than raw speed at the
// rate Mode S frames arrive (≤ a few hundred per second).
func CRC24(data []byte) uint32 {
	var rem uint32

	for _, sample := range data {
		rem ^= uint32(sample) << crcByteShift

		for range 8 { //nolint:mnd // bits per byte; obvious from the polynomial.
			if rem&crcMSB != 0 {
				rem = ((rem << 1) ^ crcPoly) & crcMask
			} else {
				rem = (rem << 1) & crcMask
			}
		}
	}

	return rem
}

// AppendCRC24 returns data with its 24-bit checksum appended in
// MSB-first byte order. Useful for synthesising Mode S frames in
// tests and for any future transmit code.
func AppendCRC24(data []byte) []byte {
	const (
		lowByteMask  uint32 = 0xff
		topByteShift uint32 = 16
		midByteShift uint32 = 8
	)

	checksum := CRC24(data)

	out := make([]byte, len(data)+ParityBytes)
	copy(out, data)
	out[len(data)+0] = byte((checksum >> topByteShift) & lowByteMask)
	out[len(data)+1] = byte((checksum >> midByteShift) & lowByteMask)
	out[len(data)+2] = byte(checksum & lowByteMask)

	return out
}

// CRCResidual returns the CRC-24 of a complete frame (message +
// parity tail). For short frames pass the full 7 bytes; for long
// frames pass the full 14. The residual is the address parity
// (AP) the producer overlaid; per-DF interpretation is documented
// at the top of this file.
func CRCResidual(frame Frame) uint32 {
	return CRC24(frame)
}
