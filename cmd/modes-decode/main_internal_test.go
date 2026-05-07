package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/hyperized/modes"
)

// Real DF 17 frames — the same fixtures the modes package's
// example tests and CRC tests use, so any drift in the underlying
// decoder surfaces here too.
const (
	// 8D40621D202CC371C32CE0576098 — DF 17, ICAO 0x40621D, TC 4
	// (callsign payload). Decodes to "KLM1023".
	hexIdentKLM1023 = "8D40621D202CC371C32CE0576098"

	// 8D8960ED58D7C2C97A12C0AB1C50 — DF 17, ICAO 0x8960ED, TC 11
	// (airborne position).
	hexAirbornePosition = "8D8960ED58D7C2C97A12C0AB1C50"
)

func TestParseConfigDefaults(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	cfg, err := parseConfig(nil, &stderr)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}

	if cfg.crcResidual != 0 {
		t.Errorf("crcResidual = %d, want 0", cfg.crcResidual)
	}
}

func TestParseConfigCRCResidual(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	cfg, err := parseConfig([]string{"-crc-residual", "0x484755"}, &stderr)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}

	if cfg.crcResidual != 0x484755 {
		t.Errorf("crcResidual = %#x, want 0x484755", cfg.crcResidual)
	}
}

func TestParseConfigBadFlag(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	if _, err := parseConfig([]string{"-nope"}, &stderr); err == nil {
		t.Fatal("parseConfig: want error, got nil")
	}
}

func TestRunVersion(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	if code := run(t.Context(), []string{"-version"}, strings.NewReader(""), &stdout, &stderr); code != exitOK {
		t.Fatalf("run = %d, want %d", code, exitOK)
	}

	if got := strings.TrimSpace(stdout.String()); got != version {
		t.Errorf("stdout = %q, want %q", got, version)
	}
}

func TestRunBadFlag(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	if code := run(t.Context(), []string{"-nope"}, strings.NewReader(""), &stdout, &stderr); code != exitUsage {
		t.Fatalf("run = %d, want %d", code, exitUsage)
	}
}

func TestRunDecodesIdentification(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	stdin := strings.NewReader(hexIdentKLM1023 + "\n")

	if code := run(t.Context(), nil, stdin, &stdout, &stderr); code != exitOK {
		t.Fatalf("run = %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}

	if !strings.Contains(stdout.String(), "40621D") {
		t.Errorf("stdout missing ICAO; got %q", stdout.String())
	}

	if !strings.Contains(stdout.String(), "df=17 tc=4 ident") {
		t.Errorf("stdout missing per-message text; got %q", stdout.String())
	}

	if !strings.Contains(stderr.String(), "decoded=1 errors=0") {
		t.Errorf("stderr missing summary; got %q", stderr.String())
	}
}

func TestRunDecodesAirbornePosition(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	stdin := strings.NewReader(hexAirbornePosition + "\n")

	if code := run(t.Context(), nil, stdin, &stdout, &stderr); code != exitOK {
		t.Fatalf("run = %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}

	if !strings.Contains(stdout.String(), "8960ED") {
		t.Errorf("stdout missing ICAO; got %q", stdout.String())
	}

	if !strings.Contains(stdout.String(), "airpos") {
		t.Errorf("stdout missing 'airpos' marker; got %q", stdout.String())
	}
}

func TestRunSkipsBlankAndCommentLines(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		"",
		"# this is a comment",
		hexIdentKLM1023,
		"  ", // whitespace-only
		"# trailing comment",
	}, "\n")

	var stdout, stderr bytes.Buffer

	if code := run(t.Context(), nil, strings.NewReader(input), &stdout, &stderr); code != exitOK {
		t.Fatalf("run = %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}

	if !strings.Contains(stderr.String(), "decoded=1 errors=0") {
		t.Errorf("stderr summary mismatch; got %q", stderr.String())
	}
}

func TestRunPropagatesErrorMarker(t *testing.T) {
	t.Parallel()

	// Demod1090 prefixes a recovered frame with a tab/space and
	// the error marker; modes-decode must strip it before
	// hex-decoding and re-emit it on the output line so the
	// operator still sees that the frame was rescued.
	input := hexIdentKLM1023 + " !errors=1\n"

	var stdout, stderr bytes.Buffer

	if code := run(t.Context(), nil, strings.NewReader(input), &stdout, &stderr); code != exitOK {
		t.Fatalf("run = %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}

	if !strings.Contains(stdout.String(), "!errors=1") {
		t.Errorf("output missing error marker; got %q", stdout.String())
	}

	if !strings.Contains(stdout.String(), "ident") {
		t.Errorf("output missing ident summary; got %q", stdout.String())
	}
}

func TestRunFailsParseAndContinues(t *testing.T) {
	t.Parallel()

	// First line: bad hex. Second line: valid. Third line: too
	// short for its DF. Total decoded=1, errors=2.
	input := strings.Join([]string{
		"not hex at all",
		hexIdentKLM1023,
		"deadbeef", // 4 bytes — DF 27 wants 14 (long DF default)
	}, "\n")

	var stdout, stderr bytes.Buffer

	if code := run(t.Context(), nil, strings.NewReader(input), &stdout, &stderr); code != exitOK {
		t.Fatalf("run = %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}

	if !strings.Contains(stderr.String(), "decoded=1 errors=2") {
		t.Errorf("stderr summary mismatch; got %q", stderr.String())
	}
}

func TestRunScannerIOError(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	if code := run(t.Context(), nil, errReader{}, &stdout, &stderr); code != exitIO {
		t.Errorf("run = %d, want %d", code, exitIO)
	}

	if !strings.Contains(stderr.String(), "scan:") {
		t.Errorf("stderr missing scan diagnostic; got %q", stderr.String())
	}
}

func TestRunCanceledMidStream(t *testing.T) {
	t.Parallel()

	// Long input; cancel after first read. The for/select inside
	// run breaks out, the scanner closes, and we exit clean.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var stdout, stderr bytes.Buffer

	if code := run(ctx, nil, strings.NewReader(hexIdentKLM1023+"\n"), &stdout, &stderr); code != exitOK {
		t.Errorf("run = %d, want %d", code, exitOK)
	}
}

func TestDecodeLineEmptyHex(t *testing.T) {
	t.Parallel()

	if _, err := decodeLine("", config{}); err == nil {
		t.Fatal("decodeLine: want error for empty input")
	}
}

func TestDecodeLineWrongLength(t *testing.T) {
	t.Parallel()

	// 8D = DF 17 (long, wants 14 bytes), supplied with only 4 bytes.
	if _, err := decodeLine("8D000000", config{}); err == nil {
		t.Fatal("decodeLine: want error for short long-DF frame")
	} else if !errors.Is(err, errFrameLength) {
		t.Errorf("decodeLine: error = %v, want errFrameLength", err)
	}
}

func TestDecodeLineWith0xPrefix(t *testing.T) {
	t.Parallel()

	out, err := decodeLine("0x"+hexIdentKLM1023, config{})
	if err != nil {
		t.Fatalf("decodeLine: %v", err)
	}

	if !strings.Contains(out, "ident") {
		t.Errorf("decodeLine output missing ident; got %q", out)
	}
}

func TestDecodeLineUnsupportedTypeCode(t *testing.T) {
	t.Parallel()

	// Frame with TC=23 (Test Message) — the modes decoder dispatcher
	// returns ErrUnsupportedTypeCode for it.
	// First byte: 0x8D = DF 17. ME byte 0 = TC<<3, so TC=23 -> 0xB8.
	frame := []byte{
		0x8D, 0x40, 0x62, 0x1D,
		0xB8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ME with TC=23
		0x00, 0x00, 0x00,
	}
	hexLine := bytesToHex(frame)

	out, err := decodeLine(hexLine, config{})
	if err != nil {
		t.Fatalf("decodeLine: %v", err)
	}

	if !strings.Contains(out, "unsupported TC") {
		t.Errorf("decodeLine output missing unsupported-TC marker; got %q", out)
	}
}

func TestFormatFrameUnknownDFFallsThrough(t *testing.T) {
	t.Parallel()

	// DF 1 has no decoder. Frame length must match its long-DF
	// default (14 bytes) for the dispatcher to accept it.
	const bogusDF byte = 1 << 3

	frame := make(modes.Frame, modes.LongFrameBytes)
	frame[0] = bogusDF

	out := formatFrame(frame, frame.DF(), 0)
	if !strings.Contains(out, "(no decoder)") {
		t.Errorf("formatFrame fallback missing; got %q", out)
	}
}

func TestSplitErrorMarker(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in     string
		hexOut string
		marker string
	}{
		"no marker":        {hexIdentKLM1023, hexIdentKLM1023, ""},
		"with marker":      {hexIdentKLM1023 + " !errors=2", hexIdentKLM1023, "!errors=2"},
		"tab separated":    {hexIdentKLM1023 + "\t!errors=1", hexIdentKLM1023, "!errors=1"},
		"trailing newline": {hexIdentKLM1023 + " !errors=0\n", hexIdentKLM1023, "!errors=0"},
	}

	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			gotHex, gotMarker := splitErrorMarker(testCase.in)
			if gotHex != testCase.hexOut {
				t.Errorf("hex = %q, want %q", gotHex, testCase.hexOut)
			}

			if gotMarker != testCase.marker {
				t.Errorf("marker = %q, want %q", gotMarker, testCase.marker)
			}
		})
	}
}

func TestFormatACASShort(t *testing.T) {
	t.Parallel()

	// DF 0 (short): 7 bytes. Bit pattern doesn't matter for the
	// formatter — we just want to exercise the call site.
	frame := modes.Frame{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	out := formatACASShort(frame, modes.ICAO(0xAABBCC))
	if !strings.Contains(out, "df=0 acas-short") {
		t.Errorf("formatACASShort output missing prefix; got %q", out)
	}
}

func TestFormatACASLong(t *testing.T) {
	t.Parallel()

	frame := make(modes.Frame, modes.LongFrameBytes)
	frame[0] = byte(modes.DFLongAirAir) << 3

	out := formatACASLong(frame, modes.ICAO(0x123456))
	if !strings.Contains(out, "df=16 acas-long") {
		t.Errorf("formatACASLong output missing prefix; got %q", out)
	}
}

func TestFormatSurveillanceAlt(t *testing.T) {
	t.Parallel()

	frame := modes.Frame{byte(modes.DFSurveillanceAlt) << 3, 0, 0, 0, 0, 0, 0}

	out := formatSurveillanceAlt(frame, modes.ICAO(0x111111))
	if !strings.Contains(out, "df=4 surv-alt") {
		t.Errorf("formatSurveillanceAlt output missing prefix; got %q", out)
	}
}

func TestFormatSurveillanceID(t *testing.T) {
	t.Parallel()

	frame := modes.Frame{byte(modes.DFSurveillanceID) << 3, 0, 0, 0, 0, 0, 0}

	out := formatSurveillanceID(frame, modes.ICAO(0x111111))
	if !strings.Contains(out, "df=5 surv-id") {
		t.Errorf("formatSurveillanceID output missing prefix; got %q", out)
	}
}

func TestFormatAllCallReply(t *testing.T) {
	t.Parallel()

	// DF 11 short frame.
	frame := modes.Frame{byte(modes.DFAllCallReply) << 3, 0xAA, 0xBB, 0xCC, 0x00, 0x00, 0x00}

	out := formatAllCallReply(frame, 0)
	if !strings.Contains(out, "df=11 allcall") {
		t.Errorf("formatAllCallReply output missing prefix; got %q", out)
	}
}

func TestFormatCommBAltitude(t *testing.T) {
	t.Parallel()

	frame := make(modes.Frame, modes.LongFrameBytes)
	frame[0] = byte(modes.DFCommBAltitude) << 3

	out := formatCommBAltitude(frame, modes.ICAO(0x222222))
	if !strings.Contains(out, "df=20 commb-alt") {
		t.Errorf("formatCommBAltitude output missing prefix; got %q", out)
	}
}

func TestFormatCommBIdentity(t *testing.T) {
	t.Parallel()

	frame := make(modes.Frame, modes.LongFrameBytes)
	frame[0] = byte(modes.DFCommBIdentity) << 3

	out := formatCommBIdentity(frame, modes.ICAO(0x222222))
	if !strings.Contains(out, "df=21 commb-id") {
		t.Errorf("formatCommBIdentity output missing prefix; got %q", out)
	}
}

func TestAltitudeFmt(t *testing.T) {
	t.Parallel()

	if got := altitudeFmt(38000, nil); got != "38000ft" {
		t.Errorf("altitudeFmt valid = %q, want 38000ft", got)
	}

	if got := altitudeFmt(0, errSyntheticAltitude); got != "?" {
		t.Errorf("altitudeFmt error branch = %q, want ?", got)
	}
}

func TestVelocityFmtSubtype1(t *testing.T) {
	t.Parallel()

	msg := modes.AirborneVelocityMessage{
		Subtype:               1,
		GroundSpeedKnots:      450,
		TrackDegrees:          90,
		GroundSpeedAvailable:  true,
		VerticalRateFeetMin:   1024,
		VerticalRateAvailable: true,
	}

	got := velocityFmt(msg)
	if !strings.Contains(got, "gs=450kt") || !strings.Contains(got, "vr=1024fpm") {
		t.Errorf("velocityFmt = %q, want gs+vr present", got)
	}
}

func TestVelocityFmtSubtypeWithoutFields(t *testing.T) {
	t.Parallel()

	msg := modes.AirborneVelocityMessage{Subtype: 3}
	got := velocityFmt(msg)

	if got != "subtype=3" {
		t.Errorf("velocityFmt = %q, want bare subtype=3", got)
	}
}

func TestSurfaceSpeedAndHeadingFmtUnavailable(t *testing.T) {
	t.Parallel()

	if got := surfaceSpeedFmt(modes.SurfacePositionMessage{}); got != "?" {
		t.Errorf("surfaceSpeedFmt unavailable = %q, want ?", got)
	}

	if got := surfaceHeadingFmt(modes.SurfacePositionMessage{}); got != "?" {
		t.Errorf("surfaceHeadingFmt unavailable = %q, want ?", got)
	}
}

func TestFormatFrameRoutesEachDF(t *testing.T) {
	t.Parallel()

	// Drive every per-DF formatter through formatFrame so the
	// switch-arm coverage matches the formatter coverage.
	tests := map[string]struct {
		df     modes.DownlinkFormat
		want   string
		long   bool
		hexME0 byte // ME byte 0 for ES variants; ignored otherwise.
	}{
		"DF 0":              {modes.DFShortAirAir, "df=0 acas-short", false, 0},
		"DF 24 (Comm-D)":    {modes.DFCommDExtendedLength, "df=24+ commd-elm", true, 0},
		"DF 4":              {modes.DFSurveillanceAlt, "df=4 surv-alt", false, 0},
		"DF 5":              {modes.DFSurveillanceID, "df=5 surv-id", false, 0},
		"DF 11":             {modes.DFAllCallReply, "df=11 allcall", false, 0},
		"DF 16":             {modes.DFLongAirAir, "df=16 acas-long", true, 0},
		"DF 18 ES":          {modes.DFNonTransponderES, "df=18", true, 0x20}, // TC 4
		"DF 20 (CommB alt)": {modes.DFCommBAltitude, "df=20 commb-alt", true, 0},
		"DF 21 (CommB id)":  {modes.DFCommBIdentity, "df=21 commb-id", true, 0},
	}

	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			length := modes.ShortFrameBytes
			if testCase.long {
				length = modes.LongFrameBytes
			}

			frame := make(modes.Frame, length)
			frame[0] = byte(testCase.df) << 3

			if testCase.long && testCase.hexME0 != 0 {
				frame[4] = testCase.hexME0
			}

			out := formatFrame(frame, testCase.df, 0)
			if !strings.Contains(out, testCase.want) {
				t.Errorf("formatFrame %s = %q, want substring %q", name, out, testCase.want)
			}
		})
	}
}

func TestFormatESDecodesEachMessageVariant(t *testing.T) {
	t.Parallel()

	// Each fixture is the wire bytes of one DF 17 frame whose
	// ME field carries a different message variant. Built by
	// hand from a known-good source (real ADS-B captures or the
	// modes package's own test fixtures) so the dispatch path is
	// exercised end-to-end.
	tests := map[string]struct {
		hexFrame string
		want     string
	}{
		"identification":    {"8D40621D202CC371C32CE0576098", "ident"},
		"airborne-position": {"8D8960ED58D7C2C97A12C0AB1C50", "airpos"},
	}

	// Synthesize variants the public package-level fixtures don't
	// hand us by zero-filling and setting just the type-code byte.
	syntheticVariants := map[string]byte{
		// TC 5..8 = surface position. TC 5 << 3 = 0x28.
		"surface-position": 0x28,
		// TC 19 = airborne velocity. 19 << 3 = 0x98.
		"airborne-velocity": 0x98,
		// TC 28 = aircraft status. 28 << 3 = 0xE0.
		"aircraft-status": 0xE0,
		// TC 29 = target state. 29 << 3 = 0xE8.
		"target-state": 0xE8,
		// TC 31 = operational status. 31 << 3 = 0xF8.
		"operational-status": 0xF8,
	}

	for name, me0 := range syntheticVariants {
		frame := make([]byte, modes.LongFrameBytes)
		frame[0] = byte(modes.DFExtendedSquitter) << 3 // DF 17
		frame[4] = me0

		tests[name] = struct {
			hexFrame string
			want     string
		}{bytesToHex(frame), markerForVariant(name)}
	}

	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			out, err := decodeLine(testCase.hexFrame, config{})
			if err != nil {
				t.Fatalf("decodeLine %s: %v", name, err)
			}

			if !strings.Contains(out, testCase.want) {
				t.Errorf("%s output = %q, want substring %q", name, out, testCase.want)
			}
		})
	}
}

func markerForVariant(name string) string {
	switch name {
	case "surface-position":
		return "surfpos"
	case "airborne-velocity":
		return "vel"
	case "aircraft-status":
		return "status"
	case "target-state":
		return "targetstate"
	case "operational-status":
		return "opstatus"
	default:
		return ""
	}
}

func TestFormatErrorBranchesReturnDiagnostic(t *testing.T) {
	t.Parallel()

	// Build a frame whose DF is 4 (surv-alt) but with a deliberately
	// invalid altitude code (M=1, Q=0) so DecodeSurveillanceAltitude
	// returns an error wrapped in the formatter's "decode:" branch.
	// Although DecodeSurveillanceAltitude returns AltitudeError on
	// the struct rather than as a function error, formatSurveillanceAlt
	// returns "decode:" only on the function error — so use a frame
	// that is too short for DecodeSurveillanceAltitude's expected
	// length to drive the err != nil path.
	short := modes.Frame{0x20, 0x00} // DF 4 but only 2 bytes; func will reject.

	out := formatSurveillanceAlt(short, modes.ICAO(0x111111))
	if !strings.Contains(out, "decode:") {
		t.Errorf("formatSurveillanceAlt error branch = %q, want 'decode:' marker", out)
	}

	out = formatSurveillanceID(short, modes.ICAO(0x111111))
	if !strings.Contains(out, "decode:") {
		t.Errorf("formatSurveillanceID error branch = %q, want 'decode:' marker", out)
	}

	out = formatACASShort(short, modes.ICAO(0))
	if !strings.Contains(out, "decode:") {
		t.Errorf("formatACASShort error branch = %q, want 'decode:' marker", out)
	}

	tooShortLong := modes.Frame{byte(modes.DFCommBAltitude) << 3, 0x00}

	out = formatCommBAltitude(tooShortLong, modes.ICAO(0))
	if !strings.Contains(out, "decode:") {
		t.Errorf("formatCommBAltitude error branch = %q, want 'decode:' marker", out)
	}

	out = formatCommBIdentity(tooShortLong, modes.ICAO(0))
	if !strings.Contains(out, "decode:") {
		t.Errorf("formatCommBIdentity error branch = %q, want 'decode:' marker", out)
	}

	out = formatACASLong(tooShortLong, modes.ICAO(0))
	if !strings.Contains(out, "decode:") {
		t.Errorf("formatACASLong error branch = %q, want 'decode:' marker", out)
	}

	out = formatAllCallReply(short, 0)
	if !strings.Contains(out, "decode:") {
		t.Errorf("formatAllCallReply error branch = %q, want 'decode:' marker", out)
	}

	out = formatCommDExtendedLength(modes.Frame{0x00}, modes.ICAO(0))
	if !strings.Contains(out, "decode:") {
		t.Errorf("formatCommDExtendedLength error branch = %q, want 'decode:' marker", out)
	}
}

func TestFormatESDecodeError(t *testing.T) {
	t.Parallel()

	// formatES takes a frame that is the right length for DF 17
	// but whose CRC is bogus / DF doesn't match. DecodeExtendedSquitter
	// returns a non-unsupported-TC error for non-DF-17/18 input.
	frame := make(modes.Frame, modes.LongFrameBytes)
	frame[0] = byte(modes.DFCommBAltitude) << 3 // DF 20 inside formatES — wrong DF.

	out := formatES(frame)
	if !strings.Contains(out, "decode:") {
		t.Errorf("formatES error branch = %q, want 'decode:' marker", out)
	}
}

func TestSurfaceSpeedAndHeadingFmtAvailable(t *testing.T) {
	t.Parallel()

	msg := modes.SurfacePositionMessage{
		GroundSpeedKnots:     17.5,
		GroundSpeedAvailable: true,
		HeadingDegrees:       270,
		HeadingAvailable:     true,
	}

	if got := surfaceSpeedFmt(msg); got != "17.5kt" {
		t.Errorf("surfaceSpeedFmt = %q, want 17.5kt", got)
	}

	if got := surfaceHeadingFmt(msg); got != "270deg" {
		t.Errorf("surfaceHeadingFmt = %q, want 270deg", got)
	}
}

// errReader is an io.Reader that always errors. Used to drive the
// scanner-failure branch of run().
type errReader struct{}

var (
	errReadFailure       = errors.New("synthetic read failure")
	errSyntheticAltitude = errors.New("synthetic altitude failure")
)

func (errReader) Read(_ []byte) (int, error) { return 0, errReadFailure }

// bytesToHex renders a byte slice as a lowercase hex string with
// no separators. Inlined helper to keep the test free of the
// encoding/hex import (already used in the production code).
func bytesToHex(b []byte) string {
	const digits = "0123456789abcdef"

	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = digits[v>>4]
		out[i*2+1] = digits[v&0x0f]
	}

	return string(out)
}

// silence unused-warning for io.EOF-style sentinels — keeps the
// import list honest while the test file is otherwise io.Reader-light.
var _ io.Reader = errReader{}
