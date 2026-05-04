package modes

import (
	"math"
	"testing"
)

// makeVelocityME synthesises a TC 19 ME payload for the
// ground-speed subsonic subtype. Inputs are signed velocities in
// knots (positive east / north) and an optional vertical rate in
// ft/min (positive up). Used by the round-trip tests.
func makeVelocityME(eastKnots, northKnots, verticalRateFtMin int) [7]byte {
	const (
		typeCode uint64 = 19
		subtype  uint64 = 1 // subsonic ground speed
		meBits          = 56
	)

	encodeAxis := func(value int) (uint64, uint64) {
		var dir uint64

		if value < 0 {
			dir = 1
			value = -value
		}

		raw := uint64(value + 1) //nolint:gosec // bounded by test inputs.

		return dir, raw
	}

	ewDir, ewRaw := encodeAxis(eastKnots)
	nsDir, nsRaw := encodeAxis(northKnots)

	var vrSign, vrRaw uint64

	if verticalRateFtMin != 0 {
		if verticalRateFtMin < 0 {
			vrSign = 1
			verticalRateFtMin = -verticalRateFtMin
		}

		vrRaw = uint64(verticalRateFtMin/64 + 1) //nolint:gosec,mnd // bounded by test inputs.
	}

	meWord := typeCode << (meBits - 5)
	meWord |= subtype << (meBits - 8)
	meWord |= ewDir << (meBits - 14)
	meWord |= ewRaw << (meBits - 24)
	meWord |= nsDir << (meBits - 25)
	meWord |= nsRaw << (meBits - 35)
	meWord |= vrSign << (meBits - 37)
	meWord |= vrRaw << (meBits - 46)

	var out [7]byte
	for index := range out {
		out[index] = byte((meWord >> uint(meBits-8-index*8)) & 0xFF)
	}

	return out
}

func TestDecodeVelocityGroundSpeedSubsonic(t *testing.T) {
	t.Parallel()

	// 300 knots east, 0 knots north → groundspeed 300, track 90°.
	mePayload := makeVelocityME(300, 0, 1024)

	msg := decodeVelocity(mePayload[:])
	if msg.Subtype != VelocitySubtypeGroundSpeedSubsonic {
		t.Errorf("Subtype = %d, want %d", msg.Subtype, VelocitySubtypeGroundSpeedSubsonic)
	}

	if !msg.GroundSpeedAvailable {
		t.Fatal("GroundSpeedAvailable = false, want true")
	}

	if math.Abs(msg.GroundSpeedKnots-300) > 1 {
		t.Errorf("GroundSpeedKnots = %.2f, want ~300", msg.GroundSpeedKnots)
	}

	if math.Abs(msg.TrackDegrees-90) > 1 {
		t.Errorf("TrackDegrees = %.2f, want ~90", msg.TrackDegrees)
	}

	if !msg.VerticalRateAvailable {
		t.Fatal("VerticalRateAvailable = false, want true")
	}

	if msg.VerticalRateFeetMin != 1024 {
		t.Errorf("VerticalRateFeetMin = %d, want 1024", msg.VerticalRateFeetMin)
	}
}

func TestDecodeVelocityNegativeAxesProduceCorrectTrack(t *testing.T) {
	t.Parallel()

	// 0 east, -200 north (200 knots southbound) → track 180°.
	mePayload := makeVelocityME(0, -200, 0)

	msg := decodeVelocity(mePayload[:])

	if !msg.GroundSpeedAvailable {
		t.Fatal("GroundSpeedAvailable = false, want true")
	}

	if math.Abs(msg.GroundSpeedKnots-200) > 1 {
		t.Errorf("GroundSpeedKnots = %.2f, want ~200", msg.GroundSpeedKnots)
	}

	if math.Abs(msg.TrackDegrees-180) > 1 {
		t.Errorf("TrackDegrees = %.2f, want ~180 (southbound)", msg.TrackDegrees)
	}
}

func TestDecodeVelocityZeroVelocityNotAvailable(t *testing.T) {
	t.Parallel()

	// 0 east, 0 north → both raw fields zero → "no value".
	var mePayload [7]byte

	mePayload[0] = (19 << 3) | 1 //nolint:mnd // TC 19, subtype 1.

	msg := decodeVelocity(mePayload[:])
	if msg.GroundSpeedAvailable {
		t.Error("GroundSpeedAvailable = true, want false (raw 0 = no value)")
	}
}

func TestDecodeVelocityVerticalRateDescent(t *testing.T) {
	t.Parallel()

	// 100 east, 100 north, descending 2048 ft/min.
	mePayload := makeVelocityME(100, 100, -2048)

	msg := decodeVelocity(mePayload[:])
	if !msg.VerticalRateAvailable {
		t.Fatal("VerticalRateAvailable = false")
	}

	if msg.VerticalRateFeetMin != -2048 {
		t.Errorf("VerticalRateFeetMin = %d, want -2048", msg.VerticalRateFeetMin)
	}
}

func TestDecodeVelocityAirspeedSubtypePopulatesStructural(t *testing.T) {
	t.Parallel()

	// TC 19 / subtype 3: airspeed subsonic. The current decoder
	// recognises the subtype but does not yet decode the
	// airspeed/heading fields — only the structural bits.
	var mePayload [7]byte

	mePayload[0] = (19 << 3) | 3 //nolint:mnd // TC 19, subtype 3.

	msg := decodeVelocity(mePayload[:])
	if msg.Subtype != VelocitySubtypeAirspeedSubsonic {
		t.Errorf("Subtype = %d, want %d", msg.Subtype, VelocitySubtypeAirspeedSubsonic)
	}

	if msg.GroundSpeedAvailable {
		t.Error("GroundSpeedAvailable = true, want false (subtype 3 has no GS)")
	}
}
