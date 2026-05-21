package modes_test

import (
	"testing"

	"github.com/hyperized/modes"
)

// Realistic frames covering one example of every DF demod1090 will
// route into this package. The DF 17 entry is the KLM1023 worked
// example from "The 1090 MHz Riddle" §4.1 (already vetted in the
// reference frame fixtures); the short DFs are synthesised with
// payloads picked to exercise their decoders without depending on
// CRC validity (decoders gate on DF + length only).
//
//nolint:gochecknoglobals // bench fixtures; hoisted so each bench iteration starts without a hex-decode setup cost.
var (
	benchExtendedSquitter = modes.Frame{
		0x8D, 0x48, 0x40, 0xD6, 0x20, 0x2C, 0xC3,
		0x71, 0xC3, 0x2C, 0xE0, 0x57, 0x60, 0x98,
	}

	benchSurveillanceAlt = modes.Frame{
		0x20, 0x00, 0x18, 0xAD, 0x12, 0x34, 0x56,
	}

	benchSurveillanceID = modes.Frame{
		0x28, 0x00, 0x18, 0xAD, 0x12, 0x34, 0x56,
	}

	benchAllCallReply = modes.Frame{
		0x5D, 0x48, 0x47, 0x55, 0x00, 0x00, 0x00,
	}

	benchACASShort = modes.Frame{
		0x00, 0x00, 0x10, 0x00, 0x12, 0x34, 0x56,
	}

	benchACASLong = modes.Frame{
		0x80, 0x00, 0x10, 0x00, 0x30, 0xA5, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x12, 0x34, 0x56,
	}

	// MB byte 0 = 0x20 so the BDS 2,0 callsign code path runs.
	benchCommBAlt = modes.Frame{
		0xA0, 0x00, 0x18, 0xAD,
		0x20, 0x2C, 0xC3, 0x71, 0xC3, 0x2C, 0xE0,
		0x12, 0x34, 0x56,
	}

	benchCommBID = modes.Frame{
		0xA8, 0x00, 0x18, 0xAD,
		0x20, 0x2C, 0xC3, 0x71, 0xC3, 0x2C, 0xE0,
		0x12, 0x34, 0x56,
	}

	benchBDS20MB = [7]byte{
		0x20, 0x2C, 0xC3, 0x71, 0xC3, 0x2C, 0xE0,
	}
)

const benchICAO modes.ICAO = 0x484755

func BenchmarkCRC24Short(b *testing.B) {
	b.ReportAllocs()

	frame := benchAllCallReply
	for b.Loop() {
		_ = modes.CRC24(frame)
	}
}

func BenchmarkCRC24Long(b *testing.B) {
	b.ReportAllocs()

	frame := benchExtendedSquitter
	for b.Loop() {
		_ = modes.CRC24(frame)
	}
}

func BenchmarkCRCResidualLong(b *testing.B) {
	b.ReportAllocs()

	frame := benchExtendedSquitter
	for b.Loop() {
		_ = modes.CRCResidual(frame)
	}
}

func BenchmarkAppendCRC24Long(b *testing.B) {
	b.ReportAllocs()

	payload := benchExtendedSquitter[:11]
	for b.Loop() {
		_ = modes.AppendCRC24(payload)
	}
}

func BenchmarkExtractDF(b *testing.B) {
	b.ReportAllocs()

	firstByte := benchExtendedSquitter[0]
	for b.Loop() {
		_ = modes.ExtractDF(firstByte)
	}
}

func BenchmarkFrameDF(b *testing.B) {
	b.ReportAllocs()

	frame := benchExtendedSquitter
	for b.Loop() {
		_ = frame.DF()
	}
}

func BenchmarkDFIsLong(b *testing.B) {
	b.ReportAllocs()

	df := modes.DFExtendedSquitter
	for b.Loop() {
		_ = df.IsLong()
	}
}

func BenchmarkDFLengthBytes(b *testing.B) {
	b.ReportAllocs()

	df := modes.DFExtendedSquitter
	for b.Loop() {
		_ = df.LengthBytes()
	}
}

func BenchmarkDecodeExtendedSquitter(b *testing.B) {
	b.ReportAllocs()

	frame := benchExtendedSquitter
	for b.Loop() {
		_, _ = modes.DecodeExtendedSquitter(frame)
	}
}

func BenchmarkDecodeAllCallReply(b *testing.B) {
	b.ReportAllocs()

	frame := benchAllCallReply
	for b.Loop() {
		_, _ = modes.DecodeAllCallReply(frame, 0)
	}
}

func BenchmarkDecodeACASShortReply(b *testing.B) {
	b.ReportAllocs()

	frame := benchACASShort
	for b.Loop() {
		_, _ = modes.DecodeACASShortReply(frame, benchICAO)
	}
}

func BenchmarkDecodeACASLongReply(b *testing.B) {
	b.ReportAllocs()

	frame := benchACASLong
	for b.Loop() {
		_, _ = modes.DecodeACASLongReply(frame, benchICAO)
	}
}

func BenchmarkDecodeSurveillanceAltitude(b *testing.B) {
	b.ReportAllocs()

	frame := benchSurveillanceAlt
	for b.Loop() {
		_, _ = modes.DecodeSurveillanceAltitude(frame, benchICAO)
	}
}

func BenchmarkDecodeSurveillanceIdentity(b *testing.B) {
	b.ReportAllocs()

	frame := benchSurveillanceID
	for b.Loop() {
		_, _ = modes.DecodeSurveillanceIdentity(frame, benchICAO)
	}
}

func BenchmarkDecodeCommBAltitude(b *testing.B) {
	b.ReportAllocs()

	frame := benchCommBAlt
	for b.Loop() {
		_, _ = modes.DecodeCommBAltitude(frame, benchICAO)
	}
}

func BenchmarkDecodeCommBIdentity(b *testing.B) {
	b.ReportAllocs()

	frame := benchCommBID
	for b.Loop() {
		_, _ = modes.DecodeCommBIdentity(frame, benchICAO)
	}
}

func BenchmarkDecodeBDS20Callsign(b *testing.B) {
	b.ReportAllocs()

	mb := benchBDS20MB
	for b.Loop() {
		_, _ = modes.DecodeBDS20Callsign(mb)
	}
}

func BenchmarkAltitudeFeet(b *testing.B) {
	b.ReportAllocs()

	const altitudeCode uint16 = 0x18AD
	for b.Loop() {
		_, _ = modes.AltitudeFeet(altitudeCode)
	}
}
