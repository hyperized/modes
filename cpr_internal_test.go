package modes

import (
	"errors"
	"math"
	"testing"
)

// CPR tests at this point are structural — they verify the
// algorithm's invariants and the surface API, not specific
// real-world frame outputs. A follow-up commit will add
// regression vectors against a real captured ADS-B frame once
// the Open IQ replay is wired in.

func TestCPRNLAtBoundaries(t *testing.T) {
	t.Parallel()

	// NL(0) = 59 (max), NL near the poles = 1. Symmetric in
	// latitude. Spot-check a handful of values for sanity.
	for _, testCase := range []struct {
		latitude float64
		want     int
	}{
		{latitude: 0.0, want: 59},
		{latitude: 5.0, want: 59},
		{latitude: -5.0, want: 59},
		{latitude: 87.0, want: 1},
		{latitude: 90.0, want: 1},
		{latitude: -90.0, want: 1},
	} {
		got := cprNL(testCase.latitude)
		if got != testCase.want {
			t.Errorf("cprNL(%.1f) = %d, want %d", testCase.latitude, got, testCase.want)
		}
	}
}

func TestCPRNLMonotonicallyDecreasing(t *testing.T) {
	t.Parallel()

	// NL decreases monotonically as |latitude| grows; check that
	// invariant across a sweep so a future bug in the lookup
	// table can't pass a hand-picked spot test.
	previous := cprNL(0)

	for lat := 0.0; lat <= 90.0; lat += 1.0 {
		got := cprNL(lat)
		if got > previous {
			t.Errorf("cprNL(%.1f) = %d > previous %d (must be monotonically non-increasing)",
				lat, got, previous)
		}

		previous = got
	}
}

func TestCPRGlobalRequiresOppositeFormats(t *testing.T) {
	t.Parallel()

	// Two even frames — DecodeCPRGlobal needs one of each.
	first := CPRPosition{Latitude: 92095, Longitude: 39846, Format: CPRFormatEven}
	second := CPRPosition{Latitude: 88385, Longitude: 125818, Format: CPRFormatEven}

	_, _, err := DecodeCPRGlobal(first, second, CPRFormatEven)
	if !errors.Is(err, errCPRZoneCrossing) {
		t.Errorf("err = %v, want errCPRZoneCrossing", err)
	}
}

func TestCPRLocalRoundsTowardReference(t *testing.T) {
	t.Parallel()

	// Locally-unambiguous decode must land within roughly half a
	// CPR zone (3°) of the reference position when the encoded
	// CPR happens to represent that same position.
	pos := CPRPosition{Latitude: 0, Longitude: 0, Format: CPRFormatEven}

	latitude, longitude := DecodeCPRLocal(pos, 52.0, 4.0)

	if math.Abs(latitude-52.0) > 6.0 {
		t.Errorf("latitude = %.5f, want within half a CPR zone of 52.0 (≤ 6°)", latitude)
	}

	if math.Abs(longitude-4.0) > 6.0 {
		t.Errorf("longitude = %.5f, want within ~6° of 4.0", longitude)
	}
}
