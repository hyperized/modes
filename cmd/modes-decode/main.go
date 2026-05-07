// Command modes-decode reads hex-encoded Mode S frames from stdin,
// one frame per line, and prints a structured one-line summary
// per frame on stdout. Designed to compose with demod1090's
// default hex output:
//
//	demod1090 | modes-decode
//	rtl-probe -capture cap.iq && demod1090 -replay-iq cap.iq | modes-decode
//
// Each input line is the wire bytes of one frame, lowercase or
// uppercase, optional `0x` prefix, optional trailing
// `!errors=N` marker (which demod1090 emits when a frame was
// rescued via single-bit error correction). Blank lines and
// lines starting with `#` are skipped. Anything that fails to
// parse is reported on stderr and counted; the process exits 0
// with the decoded/error totals on the final stderr line so a
// shell can use `2>` to keep stdout clean for pipeline use.
//
// For DFs whose ICAO is recovered from the CRC residual
// (DF 0/4/5/11/16/20/21), the residual must be supplied via
// --crc-residual or the structural fields render with ICAO=0.
// DF 17 / DF 18 carry the broadcasting AA in the message body
// and never need the flag.
package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/hyperized/modes"
)

// version is overridden at link time with -ldflags "-X main.version=...".
var version = "0.0.0-dev"

// Exit codes. 0 = clean shutdown (even if some lines failed to
// parse — that's per-line state on stderr, not the program's
// disposition). 1 = scanner / IO failure. 2 = bad CLI flag.
const (
	exitOK    = 0
	exitIO    = 1
	exitUsage = 2
)

// scannerBufferBytes caps the maximum line length the scanner
// will accept. A long Mode S frame is 14 bytes / 28 hex chars
// plus a marker; 256 bytes is a generous ceiling that still
// surfaces buffer overflows from misformed input early.
const scannerBufferBytes = 256

// errorMarkerPrefix is the substring demod1090 emits between the
// frame hex and the error-correction count. We strip it so the
// hex parser sees clean input.
const errorMarkerPrefix = "!errors="

// config holds parsed CLI flags. Pulled into a struct so
// parseConfig can be unit-tested in isolation.
type config struct {
	showVersion bool

	// crcResidual is the CRC residual the producer observed on
	// short / overlay-DF frames. demod1090's default hex output
	// does not carry it; the operator can pass it explicitly when
	// they know it (e.g. 0 for DF 11 unsolicited acquisition
	// squitters), or accept ICAO=0 for those DFs.
	crcResidual uint64
}

// parseConfig wires the Go flag package to a config. Returns the
// parsed config plus any flag.Parse error (caller maps to
// exitUsage).
func parseConfig(args []string, stderr io.Writer) (config, error) {
	var cfg config

	flagSet := flag.NewFlagSet("modes-decode", flag.ContinueOnError)
	flagSet.SetOutput(stderr)

	flagSet.BoolVar(&cfg.showVersion, "version", false, "print version and exit")
	flagSet.Uint64Var(&cfg.crcResidual, "crc-residual", 0,
		"CRC residual to supply when decoding overlay-DF frames "+
			"(DF 0/4/5/11/16/20/21). Hex or decimal. Producer-side context.")

	if err := flagSet.Parse(args); err != nil {
		return cfg, fmt.Errorf("flag parse: %w", err)
	}

	return cfg, nil
}

// run is the real entry point — main() just calls os.Exit(run(...))
// so tests can drive it with their own argv and capture stdout/
// stderr without forking a subprocess.
func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		return exitUsage
	}

	if cfg.showVersion {
		_, _ = fmt.Fprintln(stdout, version)

		return exitOK
	}

	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, scannerBufferBytes), scannerBufferBytes)

	decoded, failed := 0, 0

	for scanner.Scan() {
		if ctx.Err() != nil {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		out, err := decodeLine(line, cfg)
		if err != nil {
			failed++
			_, _ = fmt.Fprintf(stderr, "modes-decode: line %q: %v\n", line, err)

			continue
		}

		_, _ = fmt.Fprintln(stdout, out)
		decoded++
	}

	if err := scanner.Err(); err != nil {
		_, _ = fmt.Fprintf(stderr, "modes-decode: scan: %v\n", err)

		return exitIO
	}

	_, _ = fmt.Fprintf(stderr, "modes-decode: decoded=%d errors=%d\n", decoded, failed)

	return exitOK
}

// decodeLine parses a single hex line and returns its formatted
// summary. Returns the parse error verbatim so the caller can log
// it with the offending line for the operator.
func decodeLine(line string, cfg config) (string, error) {
	hexPart, errMarker := splitErrorMarker(line)

	bytes, err := hex.DecodeString(strings.TrimPrefix(hexPart, "0x"))
	if err != nil {
		return "", fmt.Errorf("hex: %w", err)
	}

	if len(bytes) == 0 {
		return "", errEmptyFrame
	}

	frame := modes.Frame(bytes)
	downlinkFormat := frame.DF()

	if want := downlinkFormat.LengthBytes(); len(frame) != want {
		return "", fmt.Errorf("%w: DF %d wants %d bytes, got %d",
			errFrameLength, downlinkFormat, want, len(frame))
	}

	//nolint:gosec // ICAO is 24 bits; truncation to uint32 matches the spec.
	out := formatFrame(frame, downlinkFormat, uint32(cfg.crcResidual))
	if errMarker != "" {
		out = out + " " + errMarker
	}

	return out, nil
}

// splitErrorMarker peels off a trailing demod1090 `!errors=N`
// marker (or any whitespace-separated tag), returning the hex
// portion and the marker (empty if absent).
func splitErrorMarker(line string) (string, string) {
	idx := strings.Index(line, errorMarkerPrefix)
	if idx < 0 {
		return line, ""
	}

	hexPart := strings.TrimSpace(line[:idx])
	marker := strings.TrimSpace(line[idx:])

	return hexPart, marker
}

// errEmptyFrame surfaces a hex-empty input line; static so callers
// can branch on it.
var errEmptyFrame = errors.New("empty frame")

// errFrameLength is the wrapped sentinel for "DF says one length,
// got another".
var errFrameLength = errors.New("frame length mismatch")

// formatFrame routes a parsed frame to the per-DF formatter.
//
//nolint:exhaustive // DFs without a per-DF formatter fall through to a structural line.
func formatFrame(frame modes.Frame, downlinkFormat modes.DownlinkFormat, crcResidual uint32) string {
	switch downlinkFormat {
	case modes.DFExtendedSquitter, modes.DFNonTransponderES:
		return formatES(frame)
	case modes.DFShortAirAir:
		return formatACASShort(frame, modes.ICAO(crcResidual))
	case modes.DFLongAirAir:
		return formatACASLong(frame, modes.ICAO(crcResidual))
	case modes.DFSurveillanceAlt:
		return formatSurveillanceAlt(frame, modes.ICAO(crcResidual))
	case modes.DFSurveillanceID:
		return formatSurveillanceID(frame, modes.ICAO(crcResidual))
	case modes.DFAllCallReply:
		return formatAllCallReply(frame, crcResidual)
	case modes.DFCommBAltitude:
		return formatCommBAltitude(frame, modes.ICAO(crcResidual))
	case modes.DFCommBIdentity:
		return formatCommBIdentity(frame, modes.ICAO(crcResidual))
	}

	// DF 24..31 (Comm-D Extended-Length Message) collapses to one
	// numeric slot in the spec. ExtractDF folds 24..31 down to 24.
	if downlinkFormat >= modes.DFCommDExtendedLength {
		return formatCommDExtendedLength(frame, modes.ICAO(crcResidual))
	}

	return fmt.Sprintf("df=%d (no decoder)", downlinkFormat)
}

// formatES dispatches a DF 17 / 18 frame through the modes
// extended-squitter decoder and returns a one-line summary.
//
//nolint:cyclop // per-Message-type dispatch; splitting hurts at-a-glance readability.
func formatES(frame modes.Frame) string {
	squitter, err := modes.DecodeExtendedSquitter(frame)
	if err != nil {
		if errors.Is(err, modes.ErrUnsupportedTypeCode) {
			return fmt.Sprintf("%06X df=%d tc=%d (unsupported TC)",
				uint32(squitter.ICAO), squitter.DF, squitter.TypeCode)
		}

		return fmt.Sprintf("decode: %v", err)
	}

	base := fmt.Sprintf("%06X df=%d tc=%d", uint32(squitter.ICAO), squitter.DF, squitter.TypeCode)

	switch msg := squitter.Message.(type) {
	case modes.IdentificationMessage:
		return fmt.Sprintf("%s ident callsign=%s set=%c category=%d",
			base, msg.Callsign, msg.CategorySet, msg.EmitterCategory)
	case modes.AirbornePositionMessage:
		return fmt.Sprintf("%s airpos alt=%s cpr=(%d,%d,%v) gnss=%v",
			base, altitudeFmt(msg.AltitudeFeet, msg.AltitudeError),
			msg.CPR.Latitude, msg.CPR.Longitude, msg.CPR.Format, msg.IsGNSSAltitude)
	case modes.SurfacePositionMessage:
		return fmt.Sprintf("%s surfpos gs=%s hdg=%s cpr=(%d,%d,%v)",
			base, surfaceSpeedFmt(msg), surfaceHeadingFmt(msg),
			msg.CPR.Latitude, msg.CPR.Longitude, msg.CPR.Format)
	case modes.AirborneVelocityMessage:
		return fmt.Sprintf("%s vel %s", base, velocityFmt(msg))
	case modes.AircraftStatusMessage:
		return fmt.Sprintf("%s status subtype=%d emergency=%d squawk=%04d",
			base, msg.Subtype, msg.EmergencyState, msg.Squawk)
	case modes.TargetStateMessage:
		return fmt.Sprintf("%s targetstate subtype=%d", base, msg.Subtype)
	case modes.OperationalStatusMessage:
		return fmt.Sprintf("%s opstatus subtype=%d", base, msg.Subtype)
	}

	return base
}

// altitudeFmt renders an altitude value with a sentinel for the
// "decoder said this is invalid" branch. Keeps the caller clean.
func altitudeFmt(feet int, err error) string {
	if err != nil {
		return "?"
	}

	return strconv.Itoa(feet) + "ft"
}

func surfaceSpeedFmt(msg modes.SurfacePositionMessage) string {
	if !msg.GroundSpeedAvailable {
		return "?"
	}

	return strconv.FormatFloat(msg.GroundSpeedKnots, 'f', 1, 64) + "kt"
}

func surfaceHeadingFmt(msg modes.SurfacePositionMessage) string {
	if !msg.HeadingAvailable {
		return "?"
	}

	return strconv.FormatFloat(msg.HeadingDegrees, 'f', 0, 64) + "deg"
}

// velocityFmt formats the variable-width Velocity payload. Ground
// speed and vertical rate each carry an availability flag because
// the decoder can recognise the variant without resolving every
// field.
func velocityFmt(msg modes.AirborneVelocityMessage) string {
	parts := make([]string, 0, 4) //nolint:mnd // 4 = subtype + 3 optional fields.

	parts = append(parts, fmt.Sprintf("subtype=%d", msg.Subtype))

	if msg.GroundSpeedAvailable {
		parts = append(parts, fmt.Sprintf("gs=%.0fkt track=%.0fdeg",
			msg.GroundSpeedKnots, msg.TrackDegrees))
	}

	if msg.VerticalRateAvailable {
		parts = append(parts, fmt.Sprintf("vr=%dfpm", msg.VerticalRateFeetMin))
	}

	return strings.Join(parts, " ")
}

func formatACASShort(frame modes.Frame, icao modes.ICAO) string {
	reply, err := modes.DecodeACASShortReply(frame, icao)
	if err != nil {
		return fmt.Sprintf("df=0 acas-short decode: %v", err)
	}

	return fmt.Sprintf("%06X df=0 acas-short alt=%s vs=%d",
		uint32(reply.ICAO), altitudeFmt(reply.AltitudeFeet, reply.AltitudeError),
		reply.VerticalStatus)
}

func formatACASLong(frame modes.Frame, icao modes.ICAO) string {
	reply, err := modes.DecodeACASLongReply(frame, icao)
	if err != nil {
		return fmt.Sprintf("df=16 acas-long decode: %v", err)
	}

	return fmt.Sprintf("%06X df=16 acas-long alt=%s vs=%d",
		uint32(icao), altitudeFmt(reply.AltitudeFeet, reply.AltitudeError),
		reply.VerticalStatus)
}

func formatSurveillanceAlt(frame modes.Frame, icao modes.ICAO) string {
	reply, err := modes.DecodeSurveillanceAltitude(frame, icao)
	if err != nil {
		return fmt.Sprintf("df=4 surv-alt decode: %v", err)
	}

	return fmt.Sprintf("%06X df=4 surv-alt alt=%s fs=%d",
		uint32(icao), altitudeFmt(reply.AltitudeFeet, reply.AltitudeError), reply.FlightStatus)
}

func formatSurveillanceID(frame modes.Frame, icao modes.ICAO) string {
	reply, err := modes.DecodeSurveillanceIdentity(frame, icao)
	if err != nil {
		return fmt.Sprintf("df=5 surv-id decode: %v", err)
	}

	return fmt.Sprintf("%06X df=5 surv-id squawk=%04d fs=%d",
		uint32(icao), reply.Squawk, reply.FlightStatus)
}

func formatAllCallReply(frame modes.Frame, crcResidual uint32) string {
	reply, err := modes.DecodeAllCallReply(frame, crcResidual)
	if err != nil {
		return fmt.Sprintf("df=11 allcall decode: %v", err)
	}

	return fmt.Sprintf("%06X df=11 allcall ca=%d unsolicited=%v",
		uint32(reply.ICAO), reply.Capability, reply.IsUnsolicited)
}

func formatCommBAltitude(frame modes.Frame, icao modes.ICAO) string {
	reply, err := modes.DecodeCommBAltitude(frame, icao)
	if err != nil {
		return fmt.Sprintf("df=20 commb-alt decode: %v", err)
	}

	return fmt.Sprintf("%06X df=20 commb-alt alt=%s fs=%d",
		uint32(icao), altitudeFmt(reply.AltitudeFeet, reply.AltitudeError), reply.FlightStatus)
}

func formatCommDExtendedLength(frame modes.Frame, icao modes.ICAO) string {
	reply, err := modes.DecodeCommDExtendedLength(frame, icao)
	if err != nil {
		return fmt.Sprintf("df=24+ commd-elm decode: %v", err)
	}

	return fmt.Sprintf("%06X df=24+ commd-elm control=%d segment=%d",
		uint32(icao), reply.Control, reply.SegmentNumber)
}

func formatCommBIdentity(frame modes.Frame, icao modes.ICAO) string {
	reply, err := modes.DecodeCommBIdentity(frame, icao)
	if err != nil {
		return fmt.Sprintf("df=21 commb-id decode: %v", err)
	}

	return fmt.Sprintf("%06X df=21 commb-id squawk=%04d fs=%d",
		uint32(icao), reply.Squawk, reply.FlightStatus)
}

func main() {
	os.Exit(realMain())
}

// realMain isolates os.Exit from the deferred cancel: gocritic's
// exitAfterDefer rule fires when os.Exit and defer share a frame,
// because the defer never runs. Splitting the entry point keeps
// the deferred cancel honoured on every return path.
func realMain() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}
