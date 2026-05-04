// Package modes implements ICAO Annex 10 Volume IV — the Mode S
// transponder downlink protocol — and RTCA DO-260B — its ADS-B
// Extended Squitter overlay. The package decodes validated Mode S
// frames (7-byte short or 14-byte long, parity-overlay-checked)
// into typed messages: aircraft identification, airborne and
// surface position via Compact Position Reporting, velocity,
// surveillance altitude / identity, ACAS coordination, and the
// Comm-A / Comm-B / Comm-D ELM channels.
//
// The package is the spec-decoding layer above demod1090's
// radio-to-bits pipeline; it has no opinion about how the bytes
// were sourced. Frames typically arrive from
// github.com/hyperized/demod1090's Demodulator output, but any
// validated `[]byte` works the same.
//
// Pure stdlib, no third-party dependencies. The decode hot path
// is allocation-free where the caller reuses a Decoder instance.
package modes
