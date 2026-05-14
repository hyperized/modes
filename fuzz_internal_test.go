package modes

import (
	"encoding/hex"
	"errors"
	"math"
	"testing"
)

// Fuzz targets for every public function that consumes wire bytes
// or other adversarial input. House style: each Decode/Parse gets
// a FuzzXxx that asserts (a) no panic, (b) when an error is
// returned for a structurally well-formed input, it wraps a known
// sentinel via %w so callers can branch with errors.Is.
//
// Seeds are pulled from the canonical fixtures in
// reference_frames_internal_test.go (Junzi Sun / pyModeS) plus a
// handful of edge-case bytes (all-zeros, all-ones, off-by-one
// lengths) so the corpus exercises the length-validation and
// dispatcher arms from the very first run.

// fuzzSeedFrames is the shared seed corpus for the DF-consuming
// fuzz targets. Hex strings are decoded once per seed at fuzz time.
//
//nolint:gochecknoglobals // immutable corpus list; pulled out to keep each FuzzXxx terse.
var fuzzSeedFrames = []string{
	// DF 17 worked examples (Junzi Sun).
	"8D4840D6202CC371C32CE0576098", // KLM1023 ident
	"8D406B902015A678D4D220AA4BDA", // EZY85MH ident
	"8D40058B58C901375147EFD09357", // airborne pos even
	"8D40058B58C904A87F402D3B8C59", // airborne pos odd
	"8D485020994409940838175B284F", // velocity sample
	"8C4841753A9A153237AEF0F275BE", // surface pos
	// DF 18 (non-transponder ES).
	"903A23FF426A4E65F7487A775D17",
	// DF 17 / TC 28 (aircraft status).
	"8DA2C1B6E112B600000000760759",
	// DF 11 acquisition squitter (constructed via makeDF11Frame —
	// hard-coded here to avoid pulling test helpers into fuzz seeds).
	"5D484755E2E0A8",
	// Edge case: empty + minimum / maximum 14-byte frames.
	"00",
	"FF",
	"00000000000000",
	"FFFFFFFFFFFFFFFFFFFFFFFFFFFF",
}

// mustDecodeHex fails the fuzz harness if the fixture hex is
// malformed. Fuzz seeds are author-provided constants — a typo
// here is a programmer bug, surfaced at corpus-load time.
func mustDecodeHex(tb testing.TB, h string) []byte {
	tb.Helper()

	out, err := hex.DecodeString(h)
	if err != nil {
		tb.Fatalf("seed hex %q: %v", h, err)
	}

	return out
}

// FuzzDecodeExtendedSquitter feeds raw bytes to the ES dispatcher.
// Asserts no panics for arbitrary input and that any returned
// error wraps one of the documented sentinels.
func FuzzDecodeExtendedSquitter(f *testing.F) {
	for _, seed := range fuzzSeedFrames {
		f.Add(mustDecodeHex(f, seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		_, err := DecodeExtendedSquitter(Frame(data))
		if err == nil {
			return
		}

		// Structurally well-formed frames must wrap one of the
		// documented sentinels so callers can branch via errors.Is.
		if !errors.Is(err, ErrWrongDF) &&
			!errors.Is(err, ErrFrameTooShort) &&
			!errors.Is(err, ErrUnsupportedTypeCode) {
			t.Fatalf("DecodeExtendedSquitter returned bare error %v (must wrap a sentinel)", err)
		}
	})
}

// FuzzDecodeAllCallReply feeds raw bytes + arbitrary residuals to
// the DF 11 decoder.
func FuzzDecodeAllCallReply(f *testing.F) {
	for _, seed := range fuzzSeedFrames {
		f.Add(mustDecodeHex(f, seed), uint32(0))
	}

	f.Fuzz(func(t *testing.T, data []byte, residual uint32) {
		_, err := DecodeAllCallReply(Frame(data), residual)
		if err == nil {
			return
		}

		if !errors.Is(err, ErrWrongDF) && !errors.Is(err, ErrFrameTooShort) {
			t.Fatalf("DecodeAllCallReply returned bare error %v (must wrap a sentinel)", err)
		}
	})
}

// FuzzAltitudeFeet drills the 13-bit altitude decoder. Every input
// is a 13-bit pattern (we mask the high bits per the function's
// contract); the function must never panic and must return one of
// the documented sentinels for the non-modern paths.
func FuzzAltitudeFeet(f *testing.F) {
	for _, seed := range []uint16{
		0,
		0x1000, // M=0, Q=0 — Gillham path
		0x0010, // M=0, Q=1 — modern, value=0
		0x0040, // M=1
		0x1FFF,
		0x0FFF,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, code uint16) {
		_, err := AltitudeFeet(code & 0x1FFF)
		if err == nil {
			return
		}

		if !errors.Is(err, ErrAltitudeMSet) && !errors.Is(err, ErrGillhamUnsupported) {
			t.Fatalf("AltitudeFeet returned bare error %v (must wrap a sentinel)", err)
		}
	})
}

// FuzzSquawkFromIdentityCode drills the 13-bit ID-code decoder.
// Documented contract: every 13-bit pattern is a syntactically
// valid squawk (the function never returns an error), and the
// resulting Squawk must always print as a 4-digit octal — i.e.
// every digit is in 0..7. Verifying that catches any future
// regression in the bit-extraction.
func FuzzSquawkFromIdentityCode(f *testing.F) {
	// Edge bit-patterns chosen to exercise corner cases of the
	// 13-bit field; not real wire frames.
	for _, seed := range []uint16{
		0,      // all-zero — squawk 0000.
		0x1FFF, // all-ones in the 13-bit field — every octal digit at 7.
		0x0F00, // upper nibbles set — high-order digits non-zero.
		0x00F0, // lower nibbles set — low-order digits non-zero.
		0xABCD, // above-the-mask noise: confirms the top 3 bits are ignored.
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, code uint16) {
		got := SquawkFromIdentityCode(code & 0x1FFF)
		// Each of the four octal digits must be 0..7.
		digits := uint16(got)
		for range 4 {
			if digits%10 > 7 { //nolint:mnd // octal-digit range check.
				t.Fatalf("SquawkFromIdentityCode(%#x) = %04d has non-octal digit", code&0x1FFF, got)
			}

			digits /= 10 //nolint:mnd // shifting one decimal digit per loop.
		}
	})
}

// FuzzDecodeBDS20Callsign drills the BDS 2,0 callsign decoder.
// Returns (callsign, ok); never errors. We assert no panic and
// that the result is ASCII-printable (the alphabet table is
// bounded; the function packs 8 chars from the lookup).
func FuzzDecodeBDS20Callsign(f *testing.F) {
	// Two real BDS 2,0 register payloads from the spec: 0x20 ||
	// 6-bit-packed callsign. The KLM1023 / EZY85MH packings are
	// derived from the public TC 4 worked examples.
	for _, seed := range [][7]byte{
		{0x20, 0x2C, 0xC3, 0x71, 0xC3, 0x2C, 0xE0}, // KLM1023
		{0x20, 0x15, 0xA6, 0x78, 0xD4, 0xD2, 0x20}, // EZY85MH
		{}, // all-zero
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, // all-ones
	} {
		buf := seed
		f.Add(buf[:])
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// The function takes a fixed-size array; zero-pad / truncate
		// at the boundary to fit the wire contract.
		var payload [7]byte
		copy(payload[:], data)

		callsign, _ := DecodeBDS20Callsign(payload)

		// Every byte must be in the documented alphabet (A-Z, 0-9,
		// space, or the '#' undefined sentinel) — the alphabet
		// builder owns this invariant.
		for index := range len(callsign) {
			char := callsign[index]
			switch {
			case char >= 'A' && char <= 'Z':
			case char >= '0' && char <= '9':
			case char == ' ' || char == modesUndefinedChar:
			default:
				t.Fatalf("DecodeBDS20Callsign produced non-alphabet byte %q (callsign=%q)", char, callsign)
			}
		}
	})
}

// FuzzDecodeCPRGlobal drills the globally-unambiguous CPR decoder
// with arbitrary 17-bit even / odd positions. The decoder must
// never panic and must surface NL-boundary crossings via
// ErrCPRZoneCrossing.
func FuzzDecodeCPRGlobal(f *testing.F) {
	// JS-§5.3 worked even/odd pair (canonical published reference).
	f.Add(uint32(92095), uint32(39846), uint32(88385), uint32(125818), uint8(0))
	// All-zero raw fields: equator-adjacent decode, mostRecent=odd.
	f.Add(uint32(0), uint32(0), uint32(0), uint32(0), uint8(1))
	// All-ones (17 bits set): far-edge raw values, mostRecent=even.
	f.Add(uint32(0x1FFFF), uint32(0x1FFFF), uint32(0x1FFFF), uint32(0x1FFFF), uint8(0))

	f.Fuzz(func(t *testing.T, evenLat, evenLon, oddLat, oddLon uint32, mostRecent uint8) {
		even := CPRPosition{
			Latitude:  evenLat & 0x1FFFF,
			Longitude: evenLon & 0x1FFFF,
			Format:    CPRFormatEven,
		}
		odd := CPRPosition{
			Latitude:  oddLat & 0x1FFFF,
			Longitude: oddLon & 0x1FFFF,
			Format:    CPRFormatOdd,
		}

		_, _, err := DecodeCPRGlobal(even, odd, CPRFormat(mostRecent))
		if err == nil {
			return
		}

		if !errors.Is(err, ErrCPRZoneCrossing) {
			t.Fatalf("DecodeCPRGlobal returned bare error %v (must wrap ErrCPRZoneCrossing)", err)
		}
	})
}

// FuzzDecodeCPRLocal drills the locally-unambiguous CPR decoder.
// Never returns an error — we just assert no panic and that the
// outputs are finite numbers (no NaN from a math.Mod edge case).
func FuzzDecodeCPRLocal(f *testing.F) {
	// JS-§5.3 even sample paired with the AMS receiver (Schiphol).
	f.Add(uint32(92095), uint32(39846), uint8(0), 52.3676, 4.9041)
	// Equator reference, odd format, zero raw fields.
	f.Add(uint32(0), uint32(0), uint8(1), 0.0, 0.0)
	// Near-polar reference, even format, all-ones raw — stresses
	// the floor / mod boundaries near the date line and the pole.
	f.Add(uint32(0x1FFFF), uint32(0x1FFFF), uint8(0), -89.9, 179.9)

	f.Fuzz(func(t *testing.T, latRaw, lonRaw uint32, format uint8, refLat, refLon float64) {
		pos := CPRPosition{
			Latitude:  latRaw & 0x1FFFF,
			Longitude: lonRaw & 0x1FFFF,
			Format:    CPRFormat(format & 1),
		}

		lat, lon := DecodeCPRLocal(pos, refLat, refLon)
		if math.IsNaN(lat) || math.IsNaN(lon) {
			t.Fatalf("DecodeCPRLocal produced NaN: (%v, %v) for pos %+v ref %v,%v",
				lat, lon, pos, refLat, refLon)
		}
	})
}

// FuzzDecodeCommDExtendedLength drills the DF 24..31 Comm-D ELM
// decoder. Returns either a parsed message or a sentinel-wrapped
// error.
func FuzzDecodeCommDExtendedLength(f *testing.F) {
	for _, seed := range fuzzSeedFrames {
		f.Add(mustDecodeHex(f, seed), uint32(0))
	}

	f.Fuzz(func(t *testing.T, data []byte, icao uint32) {
		_, err := DecodeCommDExtendedLength(Frame(data), ICAO(icao))
		if err == nil {
			return
		}

		if !errors.Is(err, ErrWrongDF) && !errors.Is(err, ErrFrameTooShort) {
			t.Fatalf("DecodeCommDExtendedLength returned bare error %v (must wrap a sentinel)", err)
		}
	})
}
