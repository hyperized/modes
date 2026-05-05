package modes

import (
	"errors"
	"math"
	"testing"
)

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

// TestCPRNLBetweenBoundaries exercises the for-loop scan path
// with a latitude that's mid-band rather than at a sentinel
// (87° polar cutoff or 0° equatorial floor).
func TestCPRNLBetweenBoundaries(t *testing.T) {
	t.Parallel()

	if got := cprNL(50.0); got < 30 || got > 50 {
		t.Errorf("cprNL(50) = %d, want a sane mid-latitude value (30..50)", got)
	}
}

func TestCPRGlobalRequiresOppositeFormats(t *testing.T) {
	t.Parallel()

	first := CPRPosition{Latitude: 92095, Longitude: 39846, Format: CPRFormatEven}
	second := CPRPosition{Latitude: 88385, Longitude: 125818, Format: CPRFormatEven}

	_, _, err := DecodeCPRGlobal(first, second, CPRFormatEven)
	if !errors.Is(err, ErrCPRZoneCrossing) {
		t.Errorf("err = %v, want ErrCPRZoneCrossing", err)
	}
}

// TestCPRGlobalRoundTripOnEncoder synthesises an even/odd pair
// from a known position via the encoder, then verifies the
// decoder recovers it within the spec's quantisation tolerance.
func TestCPRGlobalRoundTripOnEncoder(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		lat  float64
		lon  float64
	}{
		{name: "amsterdam", lat: 52.3676, lon: 4.9041},
		{name: "san_francisco", lat: 37.7749, lon: -122.4194},
		{name: "sydney", lat: -33.8688, lon: 151.2093},
		{name: "buenos_aires", lat: -34.6037, lon: -58.3816},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			even := encodeCPR(testCase.lat, testCase.lon, CPRFormatEven)
			odd := encodeCPR(testCase.lat, testCase.lon, CPRFormatOdd)

			latEven, lonEven, err := DecodeCPRGlobal(even, odd, CPRFormatEven)
			if err != nil {
				t.Fatalf("DecodeCPRGlobal even: %v", err)
			}

			if math.Abs(latEven-testCase.lat) > 0.05 {
				t.Errorf("decoded lat (even) = %.5f, want %.5f", latEven, testCase.lat)
			}

			if math.Abs(lonEven-testCase.lon) > 0.05 {
				t.Errorf("decoded lon (even) = %.5f, want %.5f", lonEven, testCase.lon)
			}

			latOdd, lonOdd, err := DecodeCPRGlobal(even, odd, CPRFormatOdd)
			if err != nil {
				t.Fatalf("DecodeCPRGlobal odd: %v", err)
			}

			if math.Abs(latOdd-testCase.lat) > 0.05 {
				t.Errorf("decoded lat (odd) = %.5f, want %.5f", latOdd, testCase.lat)
			}

			if math.Abs(lonOdd-testCase.lon) > 0.05 {
				t.Errorf("decoded lon (odd) = %.5f, want %.5f", lonOdd, testCase.lon)
			}
		})
	}
}

// TestCPRGlobalSwapsEvenOddArguments hits the input-normalisation
// path: when the caller passes the odd frame as the first
// argument, the decoder must swap them rather than reject.
func TestCPRGlobalSwapsEvenOddArguments(t *testing.T) {
	t.Parallel()

	const (
		lat = 52.0
		lon = 4.0
	)

	even := encodeCPR(lat, lon, CPRFormatEven)
	odd := encodeCPR(lat, lon, CPRFormatOdd)

	// Swap the order — pass odd first.
	latGot, lonGot, err := DecodeCPRGlobal(odd, even, CPRFormatEven)
	if err != nil {
		t.Fatalf("DecodeCPRGlobal swapped: %v", err)
	}

	if math.Abs(latGot-lat) > 0.05 {
		t.Errorf("decoded lat = %.5f, want ~%.5f", latGot, lat)
	}

	if math.Abs(lonGot-lon) > 0.05 {
		t.Errorf("decoded lon = %.5f, want ~%.5f", lonGot, lon)
	}
}

// TestCPRGlobalUnknownFormatErrors exercises the "default" arm
// of the switch where mostRecent is something other than even
// or odd — an out-of-range CPRFormat value.
func TestCPRGlobalUnknownFormatErrors(t *testing.T) {
	t.Parallel()

	even := encodeCPR(52.0, 4.0, CPRFormatEven)
	odd := encodeCPR(52.0, 4.0, CPRFormatOdd)

	if _, _, err := DecodeCPRGlobal(even, odd, CPRFormat(99)); !errors.Is(err, ErrCPRZoneCrossing) {
		t.Errorf("err = %v, want ErrCPRZoneCrossing for unknown CPRFormat", err)
	}
}

func TestCPRLocalRoundsTowardReference(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		lat    float64
		lon    float64
		format CPRFormat
	}{
		{name: "amsterdam_even", lat: 52.3676, lon: 4.9041, format: CPRFormatEven},
		{name: "amsterdam_odd", lat: 52.3676, lon: 4.9041, format: CPRFormatOdd},
		{name: "south_odd", lat: -33.8688, lon: 151.2093, format: CPRFormatOdd},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pos := encodeCPR(testCase.lat, testCase.lon, testCase.format)

			// Reference within ~1° of the encoded position so
			// the local decode lands in the right zone.
			latGot, lonGot := DecodeCPRLocal(pos, testCase.lat+0.5, testCase.lon-0.3)

			if math.Abs(latGot-testCase.lat) > 0.05 {
				t.Errorf("decoded lat = %.5f, want %.5f", latGot, testCase.lat)
			}

			if math.Abs(lonGot-testCase.lon) > 0.05 {
				t.Errorf("decoded lon = %.5f, want %.5f", lonGot, testCase.lon)
			}
		})
	}
}
