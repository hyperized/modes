package modes

import "fmt"

// Comm-D Extended Length Message (DF 24..31)
// ==========================================
//
// ICAO Annex 10 Vol IV §3.1.2.7.3. DF 24 is the "Comm-D ELM"
// downlink — a long (14-byte) frame the airborne transponder
// uses to deliver one segment of an Extended Length Message
// (up to 16 such segments form a single ELM, allowing a 1280-bit
// message). The DF field is encoded as "11" in the top two bits
// of the first byte, so the numeric DF reads back as anything in
// the range 24..31.
//
// Wire layout (112 bits):
//
//	bit  0..1   "11" prefix         (2 bits)
//	bit  2      KE (Control bit)    (1 bit)
//	bit  3..6   ND (segment number) (4 bits)
//	bit  7..87  MD (message bytes)  (10 bytes / 80 bits)
//	bit 88..111 AP (parity/address) (24 bits)
//
// The MD field's content is application-specific (link-layer
// extended message data); spec-wise it's an opaque byte string
// the originating Mode S node and the receiving ground station
// agree on. We expose it raw and leave reassembly across
// segments to the caller, since it requires per-aircraft state
// and timing tracking the decoder has no opinion about.

// CommDMessage is the decoded payload of a DF 24..31 frame.
type CommDMessage struct {
	// Control is the KE bit at position 2. 0 indicates an
	// upward-format reply to a ground interrogation; 1 is the
	// downward-format used for autonomous transmissions.
	Control uint8

	// SegmentNumber is the 4-bit ND field at positions 3..6:
	// the index of this segment within the ELM, 0..15.
	SegmentNumber uint8

	// MD is the 10-byte (80-bit) message data; opaque to this
	// package. Reassembly across segments is the caller's job.
	MD [10]byte

	ICAO ICAO
}

// DecodeCommDExtendedLength parses a DF 24..31 frame. icao is
// the addressed aircraft's ICAO recovered from the parity
// residual.
func DecodeCommDExtendedLength(frame Frame, icao ICAO) (CommDMessage, error) {
	if got := frame.DF(); got < DFCommDExtendedLength {
		return CommDMessage{}, fmt.Errorf("%w: have DF %d, want 24..31",
			errWrongDF, got)
	}

	if len(frame) != LongFrameBytes {
		return CommDMessage{}, fmt.Errorf("%w: have %d bytes, want %d for DF 24",
			errFrameTooShort, len(frame), LongFrameBytes)
	}

	const (
		controlBitMask byte = 0x20
		controlShift   byte = 5

		ndShift byte = 1
		ndMask  byte = 0x0F

		mdOffset = 1
		mdLen    = 10
	)

	out := CommDMessage{
		Control:       (frame[0] & controlBitMask) >> controlShift,
		SegmentNumber: (frame[0] >> ndShift) & ndMask,
		ICAO:          icao,
	}

	copy(out.MD[:], frame[mdOffset:mdOffset+mdLen])

	return out, nil
}
