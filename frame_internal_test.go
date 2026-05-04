package modes

import "testing"

func TestExtractDFCovers5Bits(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name      string
		firstByte byte
		want      DownlinkFormat
	}{
		{name: "DF0_zero", firstByte: 0b0000_0000, want: 0},
		{name: "DF4_top_bits_only", firstByte: 0b0010_0000, want: 4},
		{name: "DF5_bottom_bits_ignored", firstByte: 0b0010_1111, want: 5},
		{name: "DF11_all_call", firstByte: 0b0101_1000, want: 11},
		{name: "DF17_extended_squitter", firstByte: 0b1000_1101, want: 17},
		{name: "DF24_collapses_high_bits", firstByte: 0b1100_0000, want: 24},
		{name: "DF31_top_of_range", firstByte: 0b1111_1111, want: 31},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := ExtractDF(testCase.firstByte); got != testCase.want {
				t.Errorf("ExtractDF(%#08b) = %d, want %d", testCase.firstByte, got, testCase.want)
			}
		})
	}
}

func TestDownlinkFormatIsLong(t *testing.T) {
	t.Parallel()

	short := []DownlinkFormat{
		DFShortAirAir, DFSurveillanceAlt, DFSurveillanceID, DFAllCallReply,
	}

	long := []DownlinkFormat{
		DFLongAirAir, DFExtendedSquitter, DFNonTransponderES, DFMilitaryES,
		DFCommBAltitude, DFCommBIdentity, DFCommDExtendedLength,
	}

	for _, downlinkFormat := range short {
		if downlinkFormat.IsLong() {
			t.Errorf("DF %d: IsLong = true, want false", downlinkFormat)
		}

		if got := downlinkFormat.LengthBytes(); got != ShortFrameBytes {
			t.Errorf("DF %d: LengthBytes = %d, want %d", downlinkFormat, got, ShortFrameBytes)
		}
	}

	for _, downlinkFormat := range long {
		if !downlinkFormat.IsLong() {
			t.Errorf("DF %d: IsLong = false, want true", downlinkFormat)
		}

		if got := downlinkFormat.LengthBytes(); got != LongFrameBytes {
			t.Errorf("DF %d: LengthBytes = %d, want %d", downlinkFormat, got, LongFrameBytes)
		}
	}
}

// TestDF24RangeIsAllLong locks in the spec's DF 24..31 collapsing
// rule: every numeric DF value at or above 24 maps to Comm-D ELM,
// which is a long frame. ExtractDF returns the raw 5-bit value
// (24..31), but IsLong / LengthBytes must treat them all the same.
func TestDF24RangeIsAllLong(t *testing.T) {
	t.Parallel()

	for raw := DownlinkFormat(24); raw < 32; raw++ {
		if !raw.IsLong() {
			t.Errorf("DF %d: IsLong = false, want true (DF 24-31 are all Comm-D ELM)", raw)
		}
	}
}

func TestFrameDF(t *testing.T) {
	t.Parallel()

	frame := Frame{0b1000_1101, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if got := frame.DF(); got != DFExtendedSquitter {
		t.Errorf("Frame.DF = %d, want %d (DF 17)", got, DFExtendedSquitter)
	}

	if !frame.IsLong() {
		t.Error("Frame.IsLong = false for DF 17, want true")
	}
}
