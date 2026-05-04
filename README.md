# modes

[![Go Reference](https://pkg.go.dev/badge/github.com/hyperized/modes.svg)](https://pkg.go.dev/github.com/hyperized/modes)

A pure-Go implementation of **ICAO Annex 10 Volume IV** (the Mode S transponder downlink protocol) and **RTCA DO-260B** (its ADS-B Extended Squitter overlay). Decodes validated Mode S frames into typed messages — aircraft identification, airborne and surface position via Compact Position Reporting (CPR), velocity, surveillance altitude / identity, ACAS coordination, and the Comm-A / Comm-B / Comm-D ELM channels.

The package is spec-faithful, allocation-free on the hot path, and has zero third-party dependencies. It pairs naturally with [`github.com/hyperized/demod1090`](https://github.com/hyperized/demod1090) for the radio-to-bits side, but any source of validated `[]byte` Mode S frames works the same.

## Status

Scaffolding only. Feature work lands commit-by-commit; see the git log for the iteration shape. The plan is full-spec coverage: every documented Downlink Format with its sub-formats and BDS register codes for Comm-B.

## Scope

- **Frame structure**: 5-bit DF dispatch, 7- vs 14-byte length classification, CRC-24 with the Mode S generator polynomial (0xFFF409), parity-overlay validation, single-bit error correction via syndrome lookup (the demod1090 primitive moves here as the natural home).
- **DF 17 / 18 (Extended Squitter, ADS-B)**: ME field decoders for Type Codes 1–4 (callsign), 5–8 (surface position), 9–18 + 20–22 (airborne position), 19 (velocity), 23–31 (auxiliary).
- **DF 4 / 5 (surveillance)**: Gillham-coded altitude, Mode-3/A squawk, capability + flight-status sub-fields.
- **DF 11 (all-call reply)**: ICAO + capability code; the unsolicited (II=0) variant the radar uses for acquisition.
- **DF 0 / 16 (ACAS air-to-air)**: short and long airborne collision-avoidance coordination.
- **DF 20 / 21 (Comm-B)**: BDS register coverage — at minimum BDS 1,7 (capability), BDS 3,0 (ACAS resolution advisory), BDS 4,0 (selected vertical intention), BDS 5,0 (track + turn), BDS 6,0 (heading + speed), and the meteorological registers BDS 4,4 / 4,5.
- **DF 24 (Comm-D ELM)**: 80-bit Extended Length Message reassembly across linked frames.
- **CPR globally-unambiguous decoding**: standard pair-of-frames algorithm + locally-unambiguous fallback against a reference position.

## API shape (planned)

```go
// Frame is a validated Mode S downlink frame in wire order. Length
// is either 7 (short) or 14 (long) bytes; CRC has been checked by
// the producer (e.g. demod1090).
type Frame []byte

// Decoder dispatches a Frame to the appropriate per-DF parser and
// returns a typed message. Reuse a Decoder across calls — its
// CPR cache and ICAO ledger track per-aircraft state.
type Decoder struct { /* ... */ }

func New(opts ...Option) *Decoder
func (d *Decoder) Decode(frame Frame) (Message, error)
```

`Message` is a sealed interface implemented by typed values per DF — `IdentificationMessage`, `AirbornePositionMessage`, `SurveillanceAltitudeMessage`, `CommBMessage`, etc. Callers type-switch.

## Build & test

```sh
make           # fmt + vet + test (race + cover)
make lint      # golangci-lint run ./...
make cover     # produces coverage.html
```

## License

Business Source License 1.1. See `LICENSE`. Free for non-commercial use; commercial integration requires a paid license. Converts to Apache-2.0 on the change date (2036-05-05).

## References

- **ICAO Annex 10 Vol IV** — paywalled; the dump1090 / readsb source comments and the [Mode S decoder primer at mode-s.org](https://mode-s.org/) (Junzi Sun's "ADS-B Decoding Guide", free PDF) are the practical references.
- **RTCA DO-260B** — paywalled; same set of free references covers ADS-B specifics.
- [`flightaware/dump1090`](https://github.com/flightaware/dump1090) and [`wiedehopf/readsb`](https://github.com/wiedehopf/readsb) — GPL-2 reference implementations. Read for understanding; do not copy.
