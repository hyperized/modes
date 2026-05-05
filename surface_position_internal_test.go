package modes

import "testing"

func TestDecodeMovementFieldStepTable(t *testing.T) {
	t.Parallel()

	// Spot-check the boundary points where the speed-step table
	// transitions to a new step size; the per-segment math is
	// linear so the transition values are sufficient.
	// Step-table boundary values, expected outputs derived from
	// the spec formula (start of segment + step×offset). Each
	// segment's last raw value is one step below the next
	// segment's start.
	for _, testCase := range []struct {
		raw       uint
		want      float64
		available bool
	}{
		{raw: 0, want: 0, available: false},     // no info
		{raw: 1, want: 0, available: true},      // stopped
		{raw: 2, want: 0.125, available: true},  // 0.125 kt segment start
		{raw: 8, want: 0.875, available: true},  // last 0.125 kt step
		{raw: 9, want: 1.0, available: true},    // 0.25 kt segment start
		{raw: 12, want: 1.75, available: true},  // last 0.25 kt step
		{raw: 13, want: 2.0, available: true},   // 0.5 kt segment start
		{raw: 38, want: 14.5, available: true},  // last 0.5 kt step
		{raw: 39, want: 15.0, available: true},  // 1 kt segment start
		{raw: 93, want: 69.0, available: true},  // last 1 kt step
		{raw: 94, want: 70.0, available: true},  // 2 kt segment start
		{raw: 108, want: 98.0, available: true}, // last 2 kt step
		{raw: 109, want: 100.0, available: true},
		{raw: 123, want: 170.0, available: true},
		{raw: 124, want: 175.0, available: true}, // > 175 kt sentinel
		{raw: 125, want: 0, available: false},    // reserved
		{raw: 127, want: 0, available: false},    // reserved
	} {
		got, available := decodeMovementField(testCase.raw)

		if got != testCase.want {
			t.Errorf("decodeMovementField(%d) speed = %.3f, want %.3f", testCase.raw, got, testCase.want)
		}

		if available != testCase.available {
			t.Errorf("decodeMovementField(%d) available = %v, want %v",
				testCase.raw, available, testCase.available)
		}
	}
}

func TestDecodeSurfacePositionStructural(t *testing.T) {
	t.Parallel()

	// Build a TC 5 ME with movement raw = 13 (2.0 kt), heading
	// status = 1, heading raw = 64 (= 180°). CPR fields zero.
	const (
		typeCode      = 5
		movementRaw   = 13
		headingStatus = 1
		headingRaw    = 64
	)

	var mePayload [7]byte

	mePayload[0] = byte(typeCode<<3) | byte(movementRaw>>4)
	mePayload[1] = byte((movementRaw&0x0F)<<4) |
		byte(headingStatus<<3) |
		byte(headingRaw>>4)
	mePayload[2] = byte((headingRaw & 0x0F) << 4)

	msg := decodeSurfacePosition(typeCode, mePayload[:])

	if !msg.GroundSpeedAvailable {
		t.Fatal("GroundSpeedAvailable = false, want true")
	}

	if msg.GroundSpeedKnots != 2.0 {
		t.Errorf("GroundSpeedKnots = %.2f, want 2.0", msg.GroundSpeedKnots)
	}

	if !msg.HeadingAvailable {
		t.Fatal("HeadingAvailable = false, want true")
	}

	if msg.HeadingDegrees != 180.0 {
		t.Errorf("HeadingDegrees = %.2f, want 180", msg.HeadingDegrees)
	}
}
