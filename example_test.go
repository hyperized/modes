package modes_test

import (
	"errors"
	"fmt"

	"github.com/hyperized/modes"
)

// Example shows the canonical happy-path use of the package: a
// validated DF 17 frame is dispatched through the
// DecodeExtendedSquitter dispatcher, and the type-switched
// Message yields the broadcasting aircraft's callsign.
func Example() {
	// 8D40621D202CC371C32CE0576098 — DF 17, ICAO 0x40621D,
	// Aircraft Identification (TC 4, callsign payload).
	frame := modes.Frame{
		0x8D, 0x40, 0x62, 0x1D,
		0x20, 0x2C, 0xC3, 0x71, 0xC3, 0x2C, 0xE0,
		0x57, 0x60, 0x98,
	}

	squitter, err := modes.DecodeExtendedSquitter(frame)
	if err != nil {
		_, _ = fmt.Println("decode:", err)

		return
	}

	if ident, ok := squitter.Message.(modes.IdentificationMessage); ok {
		_, _ = fmt.Printf("ICAO=%06X callsign=%s\n", uint32(squitter.ICAO), ident.Callsign)
	}
}

// ExampleDecodeExtendedSquitter demonstrates the full type-switch
// dispatch consumers will write: a single decoder entry point
// produces one of several typed messages depending on the ME
// Type Code.
func ExampleDecodeExtendedSquitter() {
	frame := modes.Frame{ /* a 14-byte DF 17 / 18 frame */ }

	squitter, err := modes.DecodeExtendedSquitter(frame)
	if errors.Is(err, modes.ErrUnsupportedTypeCode) {
		// Structural fields still populated — log and move on.
		_, _ = fmt.Printf("unhandled TC %d for %06X\n", squitter.TypeCode, uint32(squitter.ICAO))

		return
	}

	if err != nil {
		_, _ = fmt.Println("decode:", err)

		return
	}

	switch msg := squitter.Message.(type) {
	case modes.IdentificationMessage:
		_, _ = fmt.Printf("ident: callsign=%s set=%c category=%d\n",
			msg.Callsign, msg.CategorySet, msg.EmitterCategory)
	case modes.AirbornePositionMessage:
		_, _ = fmt.Printf("airborne pos: alt=%dft cpr=(%d, %d, %v)\n",
			msg.AltitudeFeet, msg.CPR.Latitude, msg.CPR.Longitude, msg.CPR.Format)
	case modes.SurfacePositionMessage:
		_, _ = fmt.Printf("surface pos: speed=%.1fkt heading=%.0f° cpr=(%d, %d, %v)\n",
			msg.GroundSpeedKnots, msg.HeadingDegrees,
			msg.CPR.Latitude, msg.CPR.Longitude, msg.CPR.Format)
	case modes.AirborneVelocityMessage:
		if msg.GroundSpeedAvailable {
			_, _ = fmt.Printf("velocity: gs=%.0fkt track=%.0f° vr=%dft/min\n",
				msg.GroundSpeedKnots, msg.TrackDegrees, msg.VerticalRateFeetMin)
		}
	case modes.AircraftStatusMessage:
		_, _ = fmt.Printf("status: emergency=%d squawk=%04d\n", msg.EmergencyState, msg.Squawk)
	default:
		_, _ = fmt.Printf("unhandled %T\n", msg)
	}
}

// ExampleDecodeCPRGlobal pairs an even and odd position message
// from the same aircraft to resolve a globally-unambiguous
// latitude / longitude. The mostRecent argument identifies which
// of the two arrived later — the decoded position is the one
// that frame represents.
func ExampleDecodeCPRGlobal() {
	even := modes.CPRPosition{Latitude: 92095, Longitude: 39846, Format: modes.CPRFormatEven}
	odd := modes.CPRPosition{Latitude: 88385, Longitude: 125818, Format: modes.CPRFormatOdd}

	lat, lon, err := modes.DecodeCPRGlobal(even, odd, modes.CPRFormatEven)
	if err != nil {
		_, _ = fmt.Println("decode:", err)

		return
	}

	_, _ = fmt.Printf("position: %.4f, %.4f\n", lat, lon)
}

// ExampleDecodeCPRLocal does the locally-unambiguous decode
// against a known reference position — useful for stationary
// receivers (the receiver's own location is the natural
// reference) and for stream decoders that have a recently
// resolved position to seed against.
func ExampleDecodeCPRLocal() {
	// Position from a single CPR frame.
	pos := modes.CPRPosition{Latitude: 92095, Longitude: 39846, Format: modes.CPRFormatEven}

	// Receiver is at Schiphol Airport (52.3105°N, 4.7683°E).
	lat, lon := modes.DecodeCPRLocal(pos, 52.3105, 4.7683)

	_, _ = fmt.Printf("position: %.4f, %.4f\n", lat, lon)
}

// ExampleDecodeAllCallReply parses a DF 11 acquisition squitter.
// The interrogator field is recovered from the producer's CRC
// residual rather than the message body, so callers pass it in
// alongside the frame bytes.
func ExampleDecodeAllCallReply() {
	frame := modes.Frame{ /* 7-byte DF 11 frame */ }

	const crcResidual uint32 = 0 // 0 → unsolicited acquisition squitter

	reply, err := modes.DecodeAllCallReply(frame, crcResidual)
	if err != nil {
		_, _ = fmt.Println("decode:", err)

		return
	}

	_, _ = fmt.Printf("ICAO=%06X capability=%d unsolicited=%v\n",
		uint32(reply.ICAO), reply.Capability, reply.IsUnsolicited)
}

// ExampleCRC24 spot-shows the spec's self-cancellation property:
// CRC over a frame ending in its own CRC tail is zero. For
// address-overlaid DFs (0/4/5/16/20/21) the residual is the
// addressed aircraft's ICAO.
func ExampleCRC24() {
	body := []byte{0x8D, 0x40, 0x62, 0x1D, 0x58, 0xC3, 0x82, 0xD6, 0x90, 0xC8, 0xAC}

	frame := modes.AppendCRC24(body)

	_, _ = fmt.Printf("residual: %#06x\n", modes.CRC24(frame))
	// Output: residual: 0x000000
}
