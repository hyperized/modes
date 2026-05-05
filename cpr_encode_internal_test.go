package modes

import "math"

// encodeCPR converts a (latitude, longitude) pair in degrees into
// a CPR position with the requested format. Synthesises test
// vectors the decoder can be round-tripped against without
// depending on third-party hex constants we can't verify.
//
// Implements the spec's encoder definition (DO-260B §A.1.7.10):
//
//	yz = floor(2^17 · mod(lat, dLat) / dLat + 0.5) mod 2^17
//	xz = floor(2^17 · mod(lon, dLon) / dLon + 0.5) mod 2^17
//
// where dLat is 6° (even) or 360°/59 (odd) and dLon is
// 360°/max(NL(lat),1) (even) or 360°/max(NL(lat)-1,1) (odd).
func encodeCPR(latitude, longitude float64, format CPRFormat) CPRPosition {
	dLat := dLatEven
	if format == CPRFormatOdd {
		dLat = dLatOdd
	}

	latRaw := math.Floor(cprResolution*positiveMod(latitude, dLat)/dLat + 0.5)
	latRaw = positiveMod(latRaw, cprResolution)

	zoneCount := cprNL(latitude)

	ni := math.Max(float64(zoneCount), 1)
	if format == CPRFormatOdd {
		ni = math.Max(float64(zoneCount-1), 1)
	}

	dLon := 360.0 / ni
	lonRaw := math.Floor(cprResolution*positiveMod(longitude, dLon)/dLon + 0.5)
	lonRaw = positiveMod(lonRaw, cprResolution)

	return CPRPosition{
		Latitude:  uint32(latRaw), //nolint:gosec // bounded by mod 2^17.
		Longitude: uint32(lonRaw), //nolint:gosec // bounded by mod 2^17.
		Format:    format,
	}
}
