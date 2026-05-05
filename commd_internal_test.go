package modes

import (
	"errors"
	"testing"
)

func TestDecodeCommDStructural(t *testing.T) {
	t.Parallel()

	const wantICAO ICAO = 0x484755

	// Build a DF 24 frame: top byte = 0xC0 (DF range 24..31),
	// with KE=0, ND=5. Body bytes 1..10 are arbitrary MD data.
	body := []byte{
		0xC0 | (5 << 1), //nolint:mnd // DF prefix + ND segment 5.
		0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA,
	}

	frame := Frame(AppendCRC24(body))

	msg, err := DecodeCommDExtendedLength(frame, wantICAO)
	if err != nil {
		t.Fatalf("DecodeCommDExtendedLength: %v", err)
	}

	if msg.SegmentNumber != 5 {
		t.Errorf("SegmentNumber = %d, want 5", msg.SegmentNumber)
	}

	if msg.Control != 0 {
		t.Errorf("Control = %d, want 0", msg.Control)
	}

	wantMD := [10]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA}
	if msg.MD != wantMD {
		t.Errorf("MD = %x, want %x", msg.MD, wantMD)
	}

	if msg.ICAO != wantICAO {
		t.Errorf("ICAO = %#06x, want %#06x", msg.ICAO, wantICAO)
	}
}

func TestDecodeCommDRejectsLowerDF(t *testing.T) {
	t.Parallel()

	frame := make(Frame, LongFrameBytes)
	frame[0] = byte(DFExtendedSquitter << 3) // DF 17

	if _, err := DecodeCommDExtendedLength(frame, 0); !errors.Is(err, ErrWrongDF) {
		t.Errorf("err = %v, want ErrWrongDF", err)
	}
}

func TestDecodeCommDRejectsShortFrame(t *testing.T) {
	t.Parallel()

	frame := Frame{0xC0}
	if _, err := DecodeCommDExtendedLength(frame, 0); !errors.Is(err, ErrFrameTooShort) {
		t.Errorf("err = %v, want ErrFrameTooShort", err)
	}
}
