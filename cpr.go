package modes

import (
	"errors"
	"math"
)

// Compact Position Reporting (CPR)
// ================================
//
// ICAO Annex 10 Vol IV §3.1.2.9.4 / DO-260B §A.1.7.10. ADS-B
// position messages encode latitude and longitude as 17-bit
// "compact" CPR values rather than full floating-point degrees.
// The encoding is locally unambiguous within roughly 360 NM of
// any decoded reference and globally unambiguous when paired
// with a second message of opposite "format" (even / odd) within
// a few seconds.
//
// The encoding hashes the world into a grid of zones whose
// height is dLat and width is dLon. Two grids exist: the "even"
// grid uses dLat₀ = 360°/60 = 6°, and the "odd" grid uses
// dLat₁ = 360°/59 ≈ 6.10169°. Within a zone the 17-bit fraction
// names the position unambiguously.
//
// Globally-unambiguous decode requires both an even and an odd
// frame from the same aircraft within a 10-second window so the
// decoder can solve for the shared "latitude index" j; once j is
// known the latitude / longitude follow. The decoder picks the
// answer matching the most recently received frame.
//
// Locally-unambiguous decode uses a known reference position
// (the receiver's lat/lon, or a previously-decoded position from
// the same aircraft) to disambiguate the zone without waiting
// for a paired frame.

// CPRFormat distinguishes the two CPR grids that alternate at
// each ADS-B position transmission (~2 Hz, alternating).
type CPRFormat uint8

// Documented CPR-format values.
const (
	CPRFormatEven CPRFormat = 0
	CPRFormatOdd  CPRFormat = 1
)

// CPRPosition is a single 34-bit CPR-encoded position carrying
// 17 bits of latitude, 17 bits of longitude, and the format bit.
type CPRPosition struct {
	Latitude  uint32
	Longitude uint32
	Format    CPRFormat
}

// errCPRZoneCrossing is returned by DecodeCPRGlobal when the
// even and odd frames disagree on the longitude-zone count
// NL — meaning the aircraft crossed an NL boundary between
// transmissions and the pair can't be combined cleanly.
var errCPRZoneCrossing = errors.New("modes: CPR pair crosses NL boundary; need newer pair")

const (
	// cprResolution is the 17-bit denominator the encoded CPR
	// values divide against. Per spec: 2^17 = 131072.
	cprResolution = 131_072.0

	dLatEven = 360.0 / 60.0 // 6.0
	dLatOdd  = 360.0 / 59.0 // ≈ 6.1016949152542375
)

// DecodeCPRGlobal does the globally-unambiguous decode from a
// pair of CPR positions captured close together in time. The
// mostRecent argument identifies which of the two arrived later;
// the decoded position is the one that frame represents.
//
// Returns errCPRZoneCrossing when the even and odd frames are
// computed against different NL values (the aircraft crossed an
// NL boundary between transmissions). Callers should discard the
// older frame and retry on the next pair.
//
//nolint:nonamedreturns // (lat, lon, err) is clearer named than positional in this signature.
func DecodeCPRGlobal(even, odd CPRPosition, mostRecent CPRFormat) (latitude, longitude float64, err error) {
	if even.Format != CPRFormatEven {
		even, odd = odd, even
	}

	if odd.Format != CPRFormatOdd {
		return 0, 0, errCPRZoneCrossing
	}

	latEven := float64(even.Latitude) / cprResolution
	latOdd := float64(odd.Latitude) / cprResolution

	// Solve for the latitude index shared by both frames.
	latIndex := math.Floor(59*latEven - 60*latOdd + 0.5) //nolint:mnd // CPR §A.1.7.10 constants.

	latitudeEven := dLatEven * (math.Mod(latIndex, 60) + latEven) //nolint:mnd // 60 zones in the even grid.
	if latitudeEven >= 270 {                                      //nolint:mnd // CPR wrap threshold.
		latitudeEven -= 360 //nolint:mnd // wrap to (-90, +90].
	}

	latitudeOdd := dLatOdd * (math.Mod(latIndex, 59) + latOdd) //nolint:mnd // 59 zones in the odd grid.
	if latitudeOdd >= 270 {                                    //nolint:mnd // CPR wrap threshold.
		latitudeOdd -= 360 //nolint:mnd // wrap to (-90, +90].
	}

	if cprNL(latitudeEven) != cprNL(latitudeOdd) {
		return 0, 0, errCPRZoneCrossing
	}

	switch mostRecent {
	case CPRFormatEven:
		latitude = latitudeEven
		longitude = decodeLongitudeGlobal(even, odd, latitudeEven, CPRFormatEven)
	case CPRFormatOdd:
		latitude = latitudeOdd
		longitude = decodeLongitudeGlobal(even, odd, latitudeOdd, CPRFormatOdd)
	default:
		return 0, 0, errCPRZoneCrossing
	}

	if longitude >= 180 { //nolint:mnd // wrap longitude to (-180, +180].
		longitude -= 360 //nolint:mnd
	}

	return latitude, longitude, nil
}

func decodeLongitudeGlobal(even, odd CPRPosition, latitude float64, mostRecent CPRFormat) float64 {
	longEven := float64(even.Longitude) / cprResolution
	longOdd := float64(odd.Longitude) / cprResolution

	zoneCount := cprNL(latitude)
	lonIndex := math.Floor(longEven*float64(zoneCount-1) - longOdd*float64(zoneCount) + 0.5) //nolint:mnd

	switch mostRecent {
	case CPRFormatEven:
		niEven := math.Max(float64(zoneCount), 1)

		return (360.0 / niEven) * (math.Mod(lonIndex, niEven) + longEven) //nolint:mnd
	case CPRFormatOdd:
		niOdd := math.Max(float64(zoneCount-1), 1)

		return (360.0 / niOdd) * (math.Mod(lonIndex, niOdd) + longOdd) //nolint:mnd
	}

	return 0
}

// DecodeCPRLocal does the locally-unambiguous CPR decode against
// a known reference position (refLat, refLon in degrees). The
// reference must be within ~180 NM of the broadcasting aircraft
// for the result to be unambiguous; otherwise the decoded
// position may land in a neighbouring zone.
//
// Useful for stationary receivers (the receiver's own location
// is the natural reference) and for stream decoders that have
// a recently-decoded global position to seed against.
//
//nolint:nonamedreturns // (lat, lon) reads clearer than positional in this signature.
func DecodeCPRLocal(pos CPRPosition, refLat, refLon float64) (latitude, longitude float64) {
	dLat := dLatEven
	if pos.Format == CPRFormatOdd {
		dLat = dLatOdd
	}

	latCPR := float64(pos.Latitude) / cprResolution
	jPart := math.Floor(refLat/dLat) +
		math.Floor(0.5+math.Mod(refLat, dLat)/dLat-latCPR) //nolint:mnd

	latitude = dLat * (jPart + latCPR)

	nl := cprNL(latitude)

	ni := float64(nl)
	if pos.Format == CPRFormatOdd {
		ni = math.Max(float64(nl-1), 1)
	}

	dLon := 360.0 / ni //nolint:mnd

	longCPR := float64(pos.Longitude) / cprResolution
	mPart := math.Floor(refLon/dLon) +
		math.Floor(0.5+math.Mod(refLon, dLon)/dLon-longCPR) //nolint:mnd

	longitude = dLon * (mPart + longCPR)

	return latitude, longitude
}
