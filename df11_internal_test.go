package modes

import (
	"errors"
	"testing"
)

// makeDF11Frame builds a DF 11 frame with the given capability +
// ICAO + plain CRC (i.e. unsolicited acquisition squitter,
// II = 0). Useful for tests that exercise the decoder against
// synthetic but parity-valid frames.
func makeDF11Frame(capability CapabilityCode, icao ICAO) Frame {
	const (
		dfShift   = 3
		highShift = 16
		midShift  = 8
		byteMask  = 0xff
	)

	body := []byte{
		byte(DFAllCallReply<<dfShift) | (byte(capability) & 0x07),
		byte((icao >> highShift) & byteMask),
		byte((icao >> midShift) & byteMask),
		byte(icao & byteMask),
	}

	return Frame(AppendCRC24(body))
}

func TestDecodeAllCallReplyHappyPath(t *testing.T) {
	t.Parallel()

	const wantICAO ICAO = 0x484755

	frame := makeDF11Frame(CapabilityLevel2Airborne, wantICAO)

	reply, err := DecodeAllCallReply(frame, CRCResidual(frame))
	if err != nil {
		t.Fatalf("DecodeAllCallReply: %v", err)
	}

	if reply.ICAO != wantICAO {
		t.Errorf("ICAO = %#06x, want %#06x", reply.ICAO, wantICAO)
	}

	if reply.Capability != CapabilityLevel2Airborne {
		t.Errorf("Capability = %d, want %d (Level 2 airborne)", reply.Capability, CapabilityLevel2Airborne)
	}

	if !reply.IsUnsolicited {
		t.Error("IsUnsolicited = false, want true (clean frame, II = 0)")
	}

	if reply.Interrogator != 0 {
		t.Errorf("Interrogator = %d, want 0 (unsolicited)", reply.Interrogator)
	}
}

func TestDecodeAllCallReplyInterrogatedExtractsII(t *testing.T) {
	t.Parallel()

	// Synthesise an "II = 5" reply by passing a non-zero residual
	// straight in. The PI overlay scheme is complex (II vs SI
	// codes have different encodings); here we just verify the
	// decoder reads the low 4 bits as the interrogator ID and
	// flags non-zero residuals as solicited.
	reply, err := DecodeAllCallReply(makeDF11Frame(CapabilityLevel1, 0xABCDEF), 5)
	if err != nil {
		t.Fatalf("DecodeAllCallReply: %v", err)
	}

	if reply.Interrogator != 5 {
		t.Errorf("Interrogator = %d, want 5", reply.Interrogator)
	}

	if reply.IsUnsolicited {
		t.Error("IsUnsolicited = true, want false (residual = 5)")
	}
}

func TestDecodeAllCallReplyRejectsWrongDF(t *testing.T) {
	t.Parallel()

	frame := Frame{0x8D, 0, 0, 0, 0, 0, 0} // DF 17, short length
	if _, err := DecodeAllCallReply(frame, 0); !errors.Is(err, ErrWrongDF) {
		t.Errorf("err = %v, want ErrWrongDF", err)
	}
}

func TestDecodeAllCallReplyRejectsWrongLength(t *testing.T) {
	t.Parallel()

	const dfShift = 3

	frame := Frame{byte(DFAllCallReply << dfShift)}
	if _, err := DecodeAllCallReply(frame, 0); !errors.Is(err, ErrFrameTooShort) {
		t.Errorf("err = %v, want ErrFrameTooShort", err)
	}
}
