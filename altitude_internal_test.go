package modes

import (
	"errors"
	"testing"
)

// makeAltitudeQ1 packs an altitude in feet into the 13-bit AC
// field with M=0, Q=1 (binary 25-ft form). Used by tests to
// generate inputs with a known expected output.
func makeAltitudeQ1(altitudeFeet int) uint16 {
	const (
		altitudeOffset     = -1000
		altitudeMultiplier = 25

		topGroupNShift  uint = 5
		middleBitNShift uint = 4

		topGroupMask uint = 0x3F
		bottomMask   uint = 0x0F

		topGroupACShift  uint = 7
		middleBitACShift uint = 5
		qBitMask         uint = 1 << 4
		bottomACPosition uint = 0
	)

	n := uint((altitudeFeet - altitudeOffset) / altitudeMultiplier)

	top := (n >> topGroupNShift) & topGroupMask
	middle := (n >> middleBitNShift) & 1
	bottom := n & bottomMask

	return uint16(top<<topGroupACShift) |
		uint16(middle<<middleBitACShift) |
		uint16(qBitMask) |
		uint16(bottom<<bottomACPosition)
}

func TestAltitudeFeetQ1RoundTrip(t *testing.T) {
	t.Parallel()

	for _, want := range []int{
		-1000, // floor of the 25-ft encoding
		0,
		1000,
		10_000,
		35_000, // typical cruise
		50_000,
		50_175, // 25 ft above 50_150 — exercises a non-round value
	} {
		altitudeCode := makeAltitudeQ1(want)

		got, err := AltitudeFeet(altitudeCode)
		if err != nil {
			t.Errorf("AltitudeFeet(%#x) for %d ft: %v", altitudeCode, want, err)

			continue
		}

		if got != want {
			t.Errorf("AltitudeFeet(%#x) = %d, want %d", altitudeCode, got, want)
		}
	}
}

func TestAltitudeFeetMBitRejected(t *testing.T) {
	t.Parallel()

	const mBitMask uint16 = 1 << 6

	if _, err := AltitudeFeet(mBitMask); !errors.Is(err, ErrAltitudeMSet) {
		t.Errorf("err = %v, want ErrAltitudeMSet", err)
	}
}

func TestAltitudeFeetGillhamReturnsPlaceholderError(t *testing.T) {
	t.Parallel()

	// M=0, Q=0: legacy Gillham. Until the follow-up implements
	// the lookup machinery the decoder surfaces a clear sentinel
	// rather than guessing.
	if _, err := AltitudeFeet(0); !errors.Is(err, ErrGillhamUnsupported) {
		t.Errorf("err = %v, want ErrGillhamUnsupported", err)
	}
}
