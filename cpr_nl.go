package modes

import "math"

// cprNL returns the number of longitude zones at the given
// latitude per ICAO Annex 10 Vol IV §3.1.2.9.4.6 / DO-260B
// table A-21. NL is 59 at the equator and decreases monotonically
// to 1 near the poles; the function is symmetric about latitude 0.
//
// Two implementations exist: a closed-form involving inverse
// cosine, and a 59-entry lookup table of latitude breakpoints.
// The lookup is what every reference implementation uses (it's
// what the spec mandates as the "transition latitudes" table)
// and the closed form has a documented numerical-stability
// gotcha at the boundaries. Lookup wins on both correctness and
// speed.
//
// The breakpoint nlBoundaries[i] is the latitude at which NL
// drops from i+2 to i+1 (so for latitude < nlBoundaries[0], NL = 59).
//
//nolint:gochecknoglobals // immutable lookup table; init once.
var nlBoundaries = [...]float64{
	10.47047130, 14.82817437, 18.18626357, 21.02939493,
	23.54504487, 25.82924707, 27.93898710, 29.91135686,
	31.77209708, 33.53993436, 35.22899598, 36.85025108,
	38.41241892, 39.92256684, 41.38651832, 42.80914012,
	44.19454951, 45.54626723, 46.86733252, 48.16039128,
	49.42776439, 50.67150166, 51.89342469, 53.09516153,
	54.27817472, 55.44378444, 56.59318756, 57.72747354,
	58.84763776, 59.95459277, 61.04917774, 62.13216659,
	63.20427479, 64.26616523, 65.31845310, 66.36171008,
	67.39646774, 68.42322022, 69.44242631, 70.45451075,
	71.45986473, 72.45884545, 73.45177442, 74.43893416,
	75.42056257, 76.39684391, 77.36789461, 78.33374083,
	79.29428225, 80.24923213, 81.19801349, 82.13956981,
	83.07199445, 83.99173563, 84.89166191, 85.75541621,
	86.53536998, 87.00000000,
}

// cprNL returns the number of longitude zones at latitude.
// Range: 1 (at the poles) to 59 (at the equator).
func cprNL(latitude float64) int {
	const polarCutoff = 87.0

	abs := math.Abs(latitude)

	switch {
	case abs >= polarCutoff:
		return 1
	case abs < nlBoundaries[0]:
		return 59 //nolint:mnd // 59 zones at the equator per spec table.
	}

	// Binary search would be marginally faster but the table is
	// 58 entries; a linear scan is plenty fast and reads more
	// like the spec's "find the highest breakpoint below
	// |latitude|" definition.
	for index, breakpoint := range nlBoundaries {
		if abs < breakpoint {
			return 59 - index //nolint:mnd // NL decreases from 59 at the equator.
		}
	}

	return 1
}
