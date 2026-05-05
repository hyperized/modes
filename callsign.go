package modes

// Aircraft Identification (TC 1..4)
// =================================
//
// ICAO Annex 10 Vol IV §3.1.2.9.1. Type Codes 1..4 in the ADS-B
// ME field carry the aircraft's flight identification — what the
// pilot dialled into the FMS as the flight callsign (e.g.
// "KLM1023", "N123AB", "BAW100"). The TC value also classifies
// the aircraft's wake-vortex category combined with a 3-bit
// emitter-category sub-field:
//
//	TC | category set
//	---|--------------
//	1  | D (reserved)
//	2  | C (surface vehicles + obstacles)
//	3  | B (gliders, balloons, ultralights, …)
//	4  | A (most powered aircraft — light/small/large/heavy/…)
//
// ME field layout for TC 1..4 (56 bits, MSB-first):
//
//	bit 0..4   TC (5 bits)
//	bit 5..7   Aircraft Category sub-field (3 bits)
//	bit 8..55  Eight 6-bit characters of callsign (48 bits)
//
// Each of the eight characters is 6 bits, decoded against the
// Mode S 6-bit alphabet defined in §3.1.2.9.1.2:
//
//	value | character
//	------|----------
//	0     | undefined / pad (we render as space)
//	1..26 | A..Z
//	27..31 | undefined
//	32    | space (padding)
//	33..47 | undefined
//	48..57 | 0..9
//	58..63 | undefined
//
// The decoder maps undefined values to the Mode S "#" sentinel
// (matching dump1090 convention) so callers can tell a parse
// problem from a legitimate spacing.

// IdentificationMessage is the decoded payload of an ADS-B
// Aircraft Identification message (DF 17/18 ES, TC 1..4).
type IdentificationMessage struct {
	// CategorySet identifies which wake-vortex category set the
	// EmitterCategory belongs to: 'D' (TC 1), 'C' (TC 2),
	// 'B' (TC 3), or 'A' (TC 4).
	CategorySet byte

	// EmitterCategory is the 3-bit sub-field that, combined with
	// CategorySet, names the aircraft type per §3.1.2.9.1.1
	// (Annex 10) / DO-260B table 2-15. Value 0 in any set means
	// "no information"; specific values map to "light",
	// "small", "large", "heavy" (set A), "rotorcraft", "glider",
	// "balloon", "UAV", "obstacle", etc.
	EmitterCategory uint8

	// Callsign is the eight-character flight identification with
	// trailing spaces trimmed. Always uppercase.
	Callsign string
}

func (msg IdentificationMessage) isModesMessage() { _ = msg }

// modesAlphabet is the 64-entry lookup the ME callsign characters
// decode against. Built once so the per-character path is a
// single array index.
//
//nolint:gochecknoglobals // immutable lookup table; init once.
var modesAlphabet = buildModesAlphabet()

// modesUndefinedChar is the sentinel byte used for any 6-bit
// value the spec leaves undefined (0, 27..31, 33..47, 58..63).
// Matches dump1090 / readsb's convention so log output looks
// familiar to anyone porting from those tools.
const modesUndefinedChar byte = '#'

func buildModesAlphabet() [64]byte {
	var table [64]byte

	for index := range table {
		table[index] = modesUndefinedChar
	}

	for index := range 26 { //nolint:mnd // 26 letters in the Latin alphabet.
		table[index+1] = byte('A' + index)
	}

	const (
		spaceIndex = 32
		digitBase  = 48
	)

	table[spaceIndex] = ' '

	for digit := range 10 { //nolint:mnd // 10 decimal digits.
		table[digitBase+digit] = byte('0' + digit)
	}

	return table
}

// decodeIdentification parses a TC 1..4 ME payload into an
// IdentificationMessage. mePayload must be exactly 7 bytes
// (the structural decoder enforces that upstream); the function
// itself trusts its input.
func decodeIdentification(typeCode TypeCode, mePayload []byte) IdentificationMessage {
	const (
		categoryMask byte = 0x07

		callsignChars = 8
		bitsPerChar   = 6

		// The 48 callsign bits start 8 bits into the ME payload
		// (after the 5-bit TC + 3-bit category).
		callsignBitOffset = 8
	)

	// TC → category-set letter: TC 1 → 'D', TC 2 → 'C',
	// TC 3 → 'B', TC 4 → 'A'. Equivalent to 'A' + (4 - TC).
	categorySet := byte('A') + byte(4) - byte(typeCode) //nolint:mnd // TC 1..4 → letter offset.

	// Pack the 56-bit ME into a single uint64 (big-endian) so we
	// can slice 6-bit characters by shift+mask. The top 8 bits
	// (TC + category) get masked off when we extract characters.
	const (
		meBits          = 56
		charMask uint64 = 0x3F
	)

	var meWord uint64
	for index := range 7 {
		meWord = (meWord << 8) | uint64(mePayload[index]) //nolint:mnd // 8 bits per byte.
	}

	chars := make([]byte, 0, callsignChars)

	for charIndex := range callsignChars {
		shift := uint(meBits - callsignBitOffset - (charIndex+1)*bitsPerChar)
		value := byte((meWord >> shift) & charMask)
		chars = append(chars, modesAlphabet[value])
	}

	return IdentificationMessage{
		CategorySet:     categorySet,
		EmitterCategory: mePayload[0] & categoryMask,
		Callsign:        trimTrailingSpaces(string(chars)),
	}
}

// trimTrailingSpaces returns input with any trailing ASCII space
// characters removed. Used by callsign decoding because the
// 8-character field is space-padded for short callsigns.
func trimTrailingSpaces(input string) string {
	end := len(input)
	for end > 0 && input[end-1] == ' ' {
		end--
	}

	return input[:end]
}
