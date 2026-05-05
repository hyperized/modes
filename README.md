# modes

[![Go Reference](https://pkg.go.dev/badge/github.com/hyperized/modes.svg)](https://pkg.go.dev/github.com/hyperized/modes)

A pure-Go implementation of **ICAO Annex 10 Volume IV** (the Mode S transponder downlink protocol) and **RTCA DO-260B** (its ADS-B Extended Squitter overlay). Decodes validated Mode S frames into typed messages — aircraft identification, airborne and surface position via Compact Position Reporting (CPR), velocity, surveillance altitude / identity, ACAS coordination, Comm-A / Comm-B / Comm-D ELM channels.

Spec-faithful, allocation-free on the hot path, zero third-party dependencies. Any source of CRC-validated `[]byte` Mode S frames works the same.

## Status

Working coverage of every Downlink Format defined in the spec. Tests: 100 % statement coverage; lint clean against `golangci-lint v2.12` (`default: all`, `revive` enable-all-rules at line-length 120).

The decoded surface is broad rather than deep — every DF and most ME Type Codes have a typed decoded message; the long tail of subtype-specific sub-decoders (TC 29 selected-altitude sub-decoding, TC 31 airborne / surface sub-fields, the dozen-plus Comm-B BDS registers beyond BDS 2,0) is filled in commit-by-commit as downstream consumers ask for them. Real-frame regression vectors land alongside as captured ADS-B replay data becomes available.

## Coverage

| Downlink Format  | Decoder                            | Notes |
|------------------|------------------------------------|-------|
| DF 0             | `DecodeACASShortReply`             | Air-to-air ACAS short |
| DF 4             | `DecodeSurveillanceAltitude`       | Altitude reply |
| DF 5             | `DecodeSurveillanceIdentity`       | Identity / squawk reply |
| DF 11            | `DecodeAllCallReply`               | All-call reply (II 0..15) |
| DF 16            | `DecodeACASLongReply`              | Air-to-air ACAS long; MV exposed raw |
| DF 17 / 18       | `DecodeExtendedSquitter`           | ADS-B Extended Squitter; ME dispatched per TC |
| DF 20 / 21       | `DecodeCommBAltitude` / `Identity` | Comm-B; MB exposed raw, BDS 2,0 callsign decoded |
| DF 24..31        | `DecodeCommDExtendedLength`        | Comm-D ELM segment; reassembly is the caller's job |

ME Type Codes (DF 17 / 18):

| TC range | Message                       | Coverage |
|----------|-------------------------------|----------|
| 1..4     | Aircraft Identification       | full (callsign + emitter category set) |
| 5..8     | Surface Position              | full structure (movement / heading / CPR); resolve CPR via `DecodeCPRGlobal` or `DecodeCPRLocal` |
| 9..18    | Airborne Position (barometric)| full structure; same CPR resolution helpers |
| 19       | Airborne Velocity             | subtype 1 (subsonic ground-speed) full; subtypes 2/3/4 surface structural fields only |
| 20..22   | Airborne Position (GNSS)      | structure; per-subtype altitude lands as a follow-up |
| 28       | Aircraft Status               | subtype 1 (Emergency / Priority Status) full; subtype 2 (TCAS RA) raw |
| 29       | Target State and Status       | structural (subtype + raw) |
| 31       | Aircraft Operational Status   | structural (subtype + raw) |

## Quickstart

```go
package main

import (
	"fmt"

	"github.com/hyperized/modes"
)

func main() {
	frame := modes.Frame{ /* 7 or 14 bytes from your demodulator */ }

	switch frame.DF() {
	case modes.DFExtendedSquitter, modes.DFNonTransponderES:
		squitter, err := modes.DecodeExtendedSquitter(frame)
		if err != nil {
			fmt.Println("decode:", err)

			return
		}

		switch msg := squitter.Message.(type) {
		case modes.IdentificationMessage:
			fmt.Printf("ICAO=%06X callsign=%s category=%c%d\n",
				uint32(squitter.ICAO), msg.Callsign, msg.CategorySet, msg.EmitterCategory)
		case modes.AirbornePositionMessage:
			fmt.Printf("ICAO=%06X alt=%dft cpr=(%d, %d, %v)\n",
				uint32(squitter.ICAO), msg.AltitudeFeet,
				msg.CPR.Latitude, msg.CPR.Longitude, msg.CPR.Format)
		case modes.AirborneVelocityMessage:
			if msg.GroundSpeedAvailable {
				fmt.Printf("ICAO=%06X gs=%.0fkt track=%.0f° vr=%dft/min\n",
					uint32(squitter.ICAO), msg.GroundSpeedKnots,
					msg.TrackDegrees, msg.VerticalRateFeetMin)
			}
		}
	}
}
```

## Architecture

The public surface is two layers:

```
                       Frame ([]byte)
                          │
         ┌────────────────┴───────────────────┐
         ▼                                    ▼
   per-DF decoders                   standalone helpers
   (DecodeAllCallReply,              (CRC24, AltitudeFeet,
   DecodeSurveillanceAlt,             SquawkFromIdentityCode,
   DecodeACASShortReply,              DecodeCPRGlobal,
   DecodeExtendedSquitter,            DecodeCPRLocal,
   DecodeCommBAltitude, …)            DecodeBDS20Callsign)
         │
         ▼  (DF 17/18 only)
   decodeMEPayload
   (TC dispatch)
         │
         ▼
   Message interface
   (IdentificationMessage,
    AirbornePositionMessage,
    AirborneVelocityMessage,
    SurfacePositionMessage,
    AircraftStatusMessage,
    TargetStateMessage,
    OperationalStatusMessage)
```

The `Message` interface is sealed (callers can't add their own implementations), so type-switching on `squitter.Message` is exhaustive against the documented set. Adding a new ME Type Code is one new file — declare a struct, satisfy `isModesMessage()`, extend the dispatch chain. No public API breakage.

## Errors

Static sentinels for every recoverable failure mode; branch with `errors.Is`:

| Sentinel                          | Meaning |
|-----------------------------------|---------|
| `ErrFrameTooShort`                | Frame buffer is shorter than the DF's wire length. |
| `ErrWrongDF`                      | Decoder was handed a frame whose DF doesn't match. |
| `ErrUnsupportedTypeCode`          | ES dispatcher saw a TC without a registered decoder. Structural fields still populated. |
| `ErrCPRZoneCrossing`              | Globally-unambiguous CPR pair straddles an NL boundary; discard the older frame and retry. |
| `ErrAltitudeMSet`                 | 13-bit altitude field has M=1 (metric encoding); spec leaves the value to-be-defined. |
| `ErrGillhamUnsupported`           | Q=0 Gillham/Gray-coded altitude path; lookup table lands as a follow-up. |
| `ErrGNSSAltitudeUnsupported`      | TC 20..22 GNSS-altitude per-subtype decoding lands as a follow-up. |

## Build & test

```sh
make           # fmt + vet + test (race + cover)
make lint      # golangci-lint run ./...
make cover     # produces coverage.html
```

## License

Business Source License 1.1. See `LICENSE`. Free for non-commercial use; commercial integration requires a paid license. Converts to Apache-2.0 on the change date (2036-05-05).

## References

- **ICAO Annex 10 Vol IV** — paywalled; [Junzi Sun's "ADS-B Decoding Guide"](https://mode-s.org/) (free PDF) is the practical reference.
- **RTCA DO-260B** — paywalled; same set of free references covers ADS-B specifics.
- [`flightaware/dump1090`](https://github.com/flightaware/dump1090) and [`wiedehopf/readsb`](https://github.com/wiedehopf/readsb) — GPL-2 reference implementations. Read for understanding; do not copy.
