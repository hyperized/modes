package modes

// Frame is a Mode S downlink frame in wire order. The first 5 bits
// of the first byte are the Downlink Format (DF), which classifies
// the frame as either short (7 bytes total) or long (14 bytes).
// The trailing 3 bytes are the 24-bit parity field — see crc.go
// for what "valid" means per DF, since Mode S overlays the parity
// with an address rather than appending a plain checksum.
//
// A Frame's contract is "the producer has already validated me":
// callers pass Frame to a Decoder and trust that the CRC has been
// checked against whatever address policy applies. The decoder
// itself does no parity validation — that's the responsibility of
// whoever produced the bytes (typically a demodulator that found
// the frame on-air).
type Frame []byte

// DownlinkFormat is the 5-bit field at the head of every Mode S
// downlink frame (ICAO Annex 10 Vol IV §3.1.2.3.2.1.2). Values
// 0-23 are individually defined; 24-31 are all assigned to DF 24
// (Comm-D ELM) — the "DF" field shrinks to 2 bits when the top
// three are 11.
type DownlinkFormat uint8

// Mnemonic constants for every Downlink Format defined in
// ICAO Annex 10 Vol IV. Numeric values are protocol facts.
const (
	DFShortAirAir         DownlinkFormat = 0  // air-to-air ACAS short
	DFSurveillanceAlt     DownlinkFormat = 4  // surveillance, altitude reply
	DFSurveillanceID      DownlinkFormat = 5  // surveillance, identity (squawk) reply
	DFAllCallReply        DownlinkFormat = 11 // all-call reply
	DFLongAirAir          DownlinkFormat = 16 // air-to-air ACAS long
	DFExtendedSquitter    DownlinkFormat = 17 // Mode S transponder Extended Squitter (ADS-B)
	DFNonTransponderES    DownlinkFormat = 18 // non-transponder Extended Squitter
	DFMilitaryES          DownlinkFormat = 19 // military Extended Squitter
	DFCommBAltitude       DownlinkFormat = 20 // Comm-B, altitude reply
	DFCommBIdentity       DownlinkFormat = 21 // Comm-B, identity reply
	DFCommDExtendedLength DownlinkFormat = 24 // Comm-D Extended Length Message (ELM); covers DF 24-31
)

// Length constants for the two on-the-wire frame sizes.
const (
	// ShortFrameBytes is the wire length of DF 0/4/5/11 frames:
	// 56 bits = 7 bytes (5-bit DF + 27 bits of payload + 24-bit
	// parity).
	ShortFrameBytes = 7

	// LongFrameBytes is the wire length of DF 16/17/18/19/20/21/24
	// frames: 112 bits = 14 bytes (5-bit DF + 83 bits of payload
	// + 24-bit parity).
	LongFrameBytes = 14

	// ParityBytes is the size of the 24-bit parity field at the
	// tail of every Mode S frame. The parity is overlaid with an
	// address (ICAO, interrogator ID, …) per the per-DF rules
	// documented in crc.go.
	ParityBytes = 3
)

// downlinkFormatShift positions the 5-bit DF field at the top of
// the first byte; the bottom 3 bits hold per-DF sub-fields.
const downlinkFormatShift = 3

// ExtractDF returns the Downlink Format encoded in the top 5 bits
// of the first frame byte. DF 24 (Comm-D ELM) actually only owns
// the top 2 bits ("11" prefix), but the spec assigns the entire
// numeric range 24..31 to DF 24, so the standard right-shift
// continues to work — every value ≥ 24 is "DF 24" by definition.
func ExtractDF(firstByte byte) DownlinkFormat {
	return DownlinkFormat(firstByte >> downlinkFormatShift)
}

// IsLong reports whether the Downlink Format uses the 14-byte
// long-frame layout. Mode S splits its formats by frame length:
// short DFs (0/4/5/11) carry 56 bits; everything else (the
// long DFs plus DF 24-31 Comm-D ELM and any of the
// undefined-but-numerically-valid DF values 1-3, 6-10, 12-15)
// carries 112. The negative-list form is the natural shape — the
// long set is open-ended (DF 24-31 collapse to one slot), so
// matching against the closed short set is more honest than
// enumerating the open one.
//
//nolint:exhaustive // negative-list intentional; long DFs include the open-ended DF 24-31 range.
func (df DownlinkFormat) IsLong() bool {
	switch df {
	case DFShortAirAir, DFSurveillanceAlt, DFSurveillanceID, DFAllCallReply:
		return false
	}

	return true
}

// LengthBytes returns the on-wire byte length of a frame with this
// Downlink Format. ShortFrameBytes (7) for short DFs, LongFrameBytes
// (14) for long.
func (df DownlinkFormat) LengthBytes() int {
	if df.IsLong() {
		return LongFrameBytes
	}

	return ShortFrameBytes
}

// DF returns the Downlink Format of this frame. Panics on an
// empty frame because a zero-byte slice has no DF to return —
// upstream contract ("validated frame") rules out empty input.
func (f Frame) DF() DownlinkFormat {
	return ExtractDF(f[0])
}

// IsLong is a convenience for f.DF().IsLong().
func (f Frame) IsLong() bool { return f.DF().IsLong() }
