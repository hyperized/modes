// Package modes implements ICAO Annex 10 Volume IV — the Mode S
// transponder downlink protocol — and RTCA DO-260B — its ADS-B
// Extended Squitter overlay. The package decodes validated Mode S
// frames (7-byte short or 14-byte long, parity-overlay-checked)
// into typed messages: aircraft identification, airborne and
// surface position via Compact Position Reporting, velocity,
// surveillance altitude / identity, ACAS coordination, and the
// Comm-A / Comm-B / Comm-D ELM channels.
//
// The package has no opinion about how the bytes were sourced —
// any producer that hands it a CRC-validated []byte (a
// software-defined-radio demodulator, a stored Beast capture, an
// MLAT feed, a network bridge) works the same.
//
// # Decoding model
//
// The public surface is two layers:
//
//   - Per-DF decoders (DecodeAllCallReply, DecodeSurveillanceAltitude,
//     DecodeACASShortReply, DecodeExtendedSquitter, …) parse the
//     wire bytes into a typed result. The Extended Squitter decoder
//     additionally dispatches the ME field to a per-Type-Code decoder
//     and returns the typed payload behind the Message interface.
//
//   - Standalone helpers (CRC24, AltitudeFeet, SquawkFromIdentityCode,
//     DecodeCPRGlobal, DecodeCPRLocal, DecodeBDS20Callsign) cover
//     the cross-cutting bit-fields the per-DF decoders share.
//
// # Quickstart
//
//	frame := modes.Frame{ /* 14-byte DF 17 ES from your demodulator */ }
//
//	squitter, err := modes.DecodeExtendedSquitter(frame)
//	if err != nil { return err }
//
//	switch msg := squitter.Message.(type) {
//	case modes.IdentificationMessage:
//	    fmt.Printf("ICAO=%06X callsign=%s\n", uint32(squitter.ICAO), msg.Callsign)
//	case modes.AirbornePositionMessage:
//	    fmt.Printf("alt=%dft cpr=%v\n", msg.AltitudeFeet, msg.CPR)
//	case modes.AirborneVelocityMessage:
//	    if msg.GroundSpeedAvailable {
//	        fmt.Printf("gs=%.0fkt track=%.0f°\n", msg.GroundSpeedKnots, msg.TrackDegrees)
//	    }
//	}
//
// # Errors
//
// The package exports static error sentinels for every recoverable
// failure mode (ErrFrameTooShort, ErrWrongDF, ErrUnsupportedTypeCode,
// ErrCPRZoneCrossing, ErrAltitudeMSet, ErrGillhamUnsupported,
// ErrGNSSAltitudeUnsupported). Use errors.Is for branching; the
// wrapped errors carry context (frame length, observed DF, etc.)
// so the formatted message is human-actionable.
//
// # Performance
//
// The decode hot path is allocation-free for fixed-size payloads:
// the per-DF decoders return value types, and the ExtendedSquitter
// dispatcher's Message interface is the only allocation, bounded
// by one per frame. Stdlib-only — no third-party dependencies.
package modes
