package modes

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// Reference test frames sourced from authoritative published
// material. Every entry below has been independently verified
// against this package's own CRC24 implementation: clean
// broadcasts (DF 11 unsolicited, DF 17, DF 18) compute residual
// 0; address-overlay frames (DF 0/4/5/16/20/21) compute residual
// equal to the addressed aircraft's ICAO.
//
// Sources:
//
//   - JS-§N : Junzi Sun, "The 1090 MHz Riddle" — open-access
//     book at https://mode-s.org/1090mhz/, chapter / section N.
//     The textbook is the canonical published worked-example
//     reference for Mode S / ADS-B.
//   - pyModeS-tests : test fixtures from
//     https://github.com/junzis/pyModeS (same author as the
//     textbook; tests assert against textbook values).
//   - pyModeS-data : real-capture corpora from the same repo
//     (tests/data/sample_data_*.csv).
//
// Frames are quoted exactly as they appear in their source, then
// normalised to uppercase by the test harness.

// referenceFrame describes a known-good (or known-bad) Mode S
// hex frame plus the ground-truth values tests assert against.
//
//nolint:govet // field order favours table readability over packing.
type referenceFrame struct {
	hex      string
	source   string
	df       DownlinkFormat
	wantICAO ICAO

	// wantResidual is what CRC24(frame) should compute to.
	// Plain-CRC frames have wantResidual = 0; overlay frames
	// have wantResidual = wantICAO.
	wantResidual uint32

	// wantTC is set for DF 17/18; zero for non-ES frames.
	wantTC TypeCode

	// Decode assertion knobs. Zero values mean "not asserted".
	wantCallsign string
}

// referenceFrames is the curated set of canonical fixtures.
// Adding a new entry requires verifying its CRC residual offline
// (see /tmp/verify_frames.go in the project history). The test
// suite re-verifies on every run so a typo in either field
// surfaces as a fail.
//
//nolint:gochecknoglobals // immutable fixture table; pulled out so both Decode and CRC tests share it.
var referenceFrames = []referenceFrame{
	// DF 17 / TC 4 — Aircraft Identification.
	{
		hex:          "8D4840D6202CC371C32CE0576098",
		source:       "JS-§4.1 KLM1023 worked example",
		df:           DFExtendedSquitter,
		wantICAO:     0x4840D6,
		wantResidual: 0,
		wantTC:       4,
		wantCallsign: "KLM1023",
	},
	{
		hex:          "8D406B902015A678D4D220AA4BDA",
		source:       "JS-§9.1 EZY85MH (residual=0 worked example)",
		df:           DFExtendedSquitter,
		wantICAO:     0x406B90,
		wantResidual: 0,
		wantTC:       4,
		wantCallsign: "EZY85MH",
	},

	// DF 17 / TC 7 — Surface Position.
	{
		hex:          "8C4841753A9A153237AEF0F275BE",
		source:       "JS surface position example (Schiphol, EHAM)",
		df:           DFExtendedSquitter,
		wantICAO:     0x484175,
		wantResidual: 0,
		wantTC:       7,
	},

	// DF 17 / TC 11 — Airborne Position (baro).
	{
		hex:          "8D40058B58C901375147EFD09357",
		source:       "JS Luxembourg locally-unambiguous, even half",
		df:           DFExtendedSquitter,
		wantICAO:     0x40058B,
		wantResidual: 0,
		wantTC:       11,
	},
	{
		hex:          "8D40058B58C904A87F402D3B8C59",
		source:       "JS Luxembourg locally-unambiguous, odd half",
		df:           DFExtendedSquitter,
		wantICAO:     0x40058B,
		wantResidual: 0,
		wantTC:       11,
	},
	{
		hex:          "8D06A15358BF17FF7D4A84B47B95",
		source:       "JS high-latitude / longitude edge case",
		df:           DFExtendedSquitter,
		wantICAO:     0x06A153,
		wantResidual: 0,
		wantTC:       11,
	},

	// DF 17 / TC 19 — Airborne Velocity.
	{
		hex:          "8D485020994409940838175B284F",
		source:       "JS real-flight time series, velocity sample 1",
		df:           DFExtendedSquitter,
		wantICAO:     0x485020,
		wantResidual: 0,
		wantTC:       19,
	},
	{
		hex:          "8DA05F219B06B6AF189400CBC33F",
		source:       "JS real-flight time series, velocity sample 2",
		df:           DFExtendedSquitter,
		wantICAO:     0xA05F21,
		wantResidual: 0,
		wantTC:       19,
	},

	// DF 17 / TC 28 — Aircraft Status (Emergency / TCAS RA).
	{
		hex:          "8DA2C1B6E112B600000000760759",
		source:       "pyModeS-tests test_bds62 emergency-status frame",
		df:           DFExtendedSquitter,
		wantICAO:     0xA2C1B6,
		wantResidual: 0,
		wantTC:       28,
	},

	// DF 17 / TC 29 — Target State and Status (DO-260B).
	{
		hex:          "8DA05629EA21485CBF3F8CADAEEB",
		source:       "pyModeS-tests test_bds62 target-state subtype 1",
		df:           DFExtendedSquitter,
		wantICAO:     0xA05629,
		wantResidual: 0,
		wantTC:       29,
	},

	// DF 18 — Non-transponder ES (CF=0 / surface beacon).
	{
		hex:          "903a23ff426a4e65f7487a775d17",
		source:       "pyModeS-data DF 18 surface (Toulouse-Blagnac)",
		df:           DFNonTransponderES,
		wantICAO:     0x3A23FF,
		wantResidual: 0,
		wantTC:       8,
	},
	{
		hex:          "903a33ff40100858d34ff3cce976",
		source:       "pyModeS-data DF 18 surface BDS 0,6 movement",
		df:           DFNonTransponderES,
		wantICAO:     0x3A33FF,
		wantResidual: 0,
		wantTC:       8,
	},
}

// TestReferenceFramesCRC asserts every fixture's CRC residual
// matches its declared wantResidual. Any divergence means
// either the hex was transcribed wrong or the CRC24 table /
// generator polynomial regressed.
func TestReferenceFramesCRC(t *testing.T) {
	t.Parallel()

	for _, ref := range referenceFrames {
		t.Run(ref.source, func(t *testing.T) {
			t.Parallel()

			frame := decodeHex(t, ref.hex)
			got := CRC24(frame)

			if got != ref.wantResidual {
				t.Errorf(
					"CRC24(%s) = %#06x, want %#06x — fixture or implementation drift",
					strings.ToUpper(ref.hex), got, ref.wantResidual,
				)
			}
		})
	}
}

// TestReferenceFramesDecode runs every fixture through
// DecodeExtendedSquitter and asserts the structural fields
// (ICAO, TypeCode, optional Callsign) match the published
// values. Catches regressions in the per-TC dispatchers and
// payload extractors without needing synthetic frame builders.
func TestReferenceFramesDecode(t *testing.T) {
	t.Parallel()

	for _, ref := range referenceFrames {
		// Only assert ES-specific fields for DF 17/18.
		if ref.df != DFExtendedSquitter && ref.df != DFNonTransponderES {
			continue
		}

		t.Run(ref.source, func(t *testing.T) {
			t.Parallel()
			assertReferenceDecode(t, ref)
		})
	}
}

// assertReferenceDecode runs DecodeExtendedSquitter on a fixture
// and checks every populated wantX field. Hoisted out of the
// loop body so the cognitive complexity stays low and so the
// per-message assertions can be extended without bloating the
// test runner.
func assertReferenceDecode(t *testing.T, ref referenceFrame) {
	t.Helper()

	frame := Frame(decodeHex(t, ref.hex))

	squitter, err := DecodeExtendedSquitter(frame)

	// Some TCs (28, 29) decode the structural fields but have no
	// per-TC payload handler yet — that returns
	// ErrUnsupportedTypeCode while still populating
	// DF/ICAO/TypeCode. We accept both outcomes.
	if err != nil && !errIsUnsupportedTC(err) {
		t.Fatalf("DecodeExtendedSquitter: %v", err)
	}

	if squitter.DF != ref.df {
		t.Errorf("DF = %d, want %d", squitter.DF, ref.df)
	}

	if squitter.ICAO != ref.wantICAO {
		t.Errorf("ICAO = %#06x, want %#06x", squitter.ICAO, ref.wantICAO)
	}

	if squitter.TypeCode != ref.wantTC {
		t.Errorf("TypeCode = %d, want %d", squitter.TypeCode, ref.wantTC)
	}

	if ref.wantCallsign != "" {
		assertCallsign(t, squitter.Message, ref.wantCallsign)
	}
}

// assertCallsign extracts the IdentificationMessage from the
// decoded ES message and compares its Callsign against want.
func assertCallsign(t *testing.T, msg Message, want string) {
	t.Helper()

	ident, ok := msg.(IdentificationMessage)
	if !ok {
		t.Fatalf("Message is %T, want IdentificationMessage", msg)
	}

	if ident.Callsign != want {
		t.Errorf("Callsign = %q, want %q", ident.Callsign, want)
	}
}

// decodeHex parses a hex string (case-insensitive) into bytes,
// failing the test cleanly if the fixture has a typo.
func decodeHex(t *testing.T, hexStr string) []byte {
	t.Helper()

	bytes, err := hex.DecodeString(strings.ToUpper(hexStr))
	if err != nil {
		t.Fatalf("hex.DecodeString(%q): %v", hexStr, err)
	}

	return bytes
}

// errIsUnsupportedTC reports whether err is or wraps
// ErrUnsupportedTypeCode. The dispatcher wraps the sentinel via
// %w, so errors.Is is the canonical predicate.
func errIsUnsupportedTC(err error) bool {
	return errors.Is(err, ErrUnsupportedTypeCode)
}
