package modes

import "fmt"

// Surveillance frames (DF 4, DF 5)
// =================================
//
// ICAO Annex 10 Vol IV §3.1.2.6.5 (DF 4 altitude reply) and
// §3.1.2.6.7 (DF 5 identity reply). Both are short (56-bit / 7-
// byte) frames with the same FS / DR / UM header and an
// AC-or-ID payload in the same 13-bit slot. The parity tail is
// overlaid with the addressed aircraft's ICAO — the receiver
// can't validate the frame without knowing that address (or
// guessing from a roster).
//
// Wire layout (32-bit prefix before the 24-bit AP):
//
//	bit 31..27 = DF (5 bits)
//	bit 26..24 = FS (3 bits, flight status)
//	bit 23..19 = DR (5 bits, downlink request)
//	bit 18..13 = UM (6 bits, utility message)
//	bit 12..0  = AC (DF 4) or ID (DF 5) — 13 bits

// FlightStatus encodes the transponder's current operational
// state: airborne vs on-ground, whether the IFR alert flag was
// raised, and whether the pilot pushed the SPI ("ident") button.
// Values per §3.1.2.6.5.1.
type FlightStatus uint8

// FlightStatus values per §3.1.2.6.5.1.
const (
	FlightStatusAirborne         FlightStatus = 0
	FlightStatusOnGround         FlightStatus = 1
	FlightStatusAirborneAlert    FlightStatus = 2
	FlightStatusOnGroundAlert    FlightStatus = 3
	FlightStatusSPIAlertEither   FlightStatus = 4
	FlightStatusSPINoAlertEither FlightStatus = 5
	FlightStatusFSReservedValue6 FlightStatus = 6
	FlightStatusFSNotAssigned7   FlightStatus = 7
)

// SurveillanceAltitude is the decoded form of a DF 4 frame:
// flight status + altitude + the addressed aircraft's ICAO
// (recovered from the CRC residual the producer computed).
type SurveillanceAltitude struct {
	FlightStatus    FlightStatus
	DownlinkRequest uint8
	UtilityMessage  uint8
	AltitudeFeet    int
	AltitudeError   error // non-nil if the AC field could not be decoded (e.g. M=1, Q=0)
	ICAO            ICAO
}

// SurveillanceIdentity is the decoded form of a DF 5 frame:
// flight status + Mode 3/A squawk + the addressed ICAO.
type SurveillanceIdentity struct {
	FlightStatus    FlightStatus
	DownlinkRequest uint8
	UtilityMessage  uint8
	Squawk          Squawk
	ICAO            ICAO
}

// surveillanceHeader is the 4-byte prefix shared by DF 4 and DF 5.
// The 13-bit payload field is returned as a uint16 with bits
// 12..0 set; the caller decides whether to treat it as AC (DF 4)
// or ID (DF 5).
type surveillanceHeader struct {
	flightStatus    FlightStatus
	downlinkRequest uint8
	utilityMessage  uint8
	payload13       uint16
}

// extractSurveillanceHeader pulls the FS / DR / UM / AC-or-ID
// fields from the 4-byte prefix of a short surveillance frame.
func extractSurveillanceHeader(frame Frame) surveillanceHeader {
	const (
		flightStatusMask uint8 = 0x07

		drBitShift uint8 = 3
		drMask     uint8 = 0x1F

		// UM straddles bytes 1 and 2: top 3 bits are byte 1 low
		// nibble, bottom 3 bits are byte 2 top.
		umHighShift uint8  = 3
		umHighMask  uint8  = 0x07
		umLowShift  uint8  = 5
		umLowMask   uint8  = 0x07
		acHighShift uint16 = 8
		acMask      uint16 = 0x1FFF
	)

	return surveillanceHeader{
		flightStatus:    FlightStatus(frame[0] & flightStatusMask),
		downlinkRequest: (frame[1] >> drBitShift) & drMask,
		utilityMessage:  ((frame[1] & umHighMask) << umHighShift) | ((frame[2] >> umLowShift) & umLowMask),
		payload13:       (uint16(frame[2])<<acHighShift | uint16(frame[3])) & acMask,
	}
}

// DecodeSurveillanceAltitude parses a DF 4 frame. icao is the
// addressed aircraft's ICAO, which the producer recovered from
// the parity residual; surveillance frames overlay parity with
// the addressee's ICAO so the receiver has no way to learn it
// from the message body alone.
func DecodeSurveillanceAltitude(frame Frame, icao ICAO) (SurveillanceAltitude, error) {
	// Length-check first: Frame.DF() panics on an empty slice.
	if len(frame) != ShortFrameBytes {
		return SurveillanceAltitude{}, fmt.Errorf("%w: have %d bytes, want %d for DF 4",
			ErrFrameTooShort, len(frame), ShortFrameBytes)
	}

	if got := frame.DF(); got != DFSurveillanceAlt {
		return SurveillanceAltitude{}, fmt.Errorf("%w: have DF %d, want %d",
			ErrWrongDF, got, DFSurveillanceAlt)
	}

	header := extractSurveillanceHeader(frame)

	altitude, err := AltitudeFeet(header.payload13)

	return SurveillanceAltitude{
		FlightStatus:    header.flightStatus,
		DownlinkRequest: header.downlinkRequest,
		UtilityMessage:  header.utilityMessage,
		AltitudeFeet:    altitude,
		AltitudeError:   err,
		ICAO:            icao,
	}, nil
}

// DecodeSurveillanceIdentity parses a DF 5 frame. Same icao
// contract as DecodeSurveillanceAltitude.
func DecodeSurveillanceIdentity(frame Frame, icao ICAO) (SurveillanceIdentity, error) {
	// Length-check first: Frame.DF() panics on an empty slice.
	if len(frame) != ShortFrameBytes {
		return SurveillanceIdentity{}, fmt.Errorf("%w: have %d bytes, want %d for DF 5",
			ErrFrameTooShort, len(frame), ShortFrameBytes)
	}

	if got := frame.DF(); got != DFSurveillanceID {
		return SurveillanceIdentity{}, fmt.Errorf("%w: have DF %d, want %d",
			ErrWrongDF, got, DFSurveillanceID)
	}

	header := extractSurveillanceHeader(frame)

	return SurveillanceIdentity{
		FlightStatus:    header.flightStatus,
		DownlinkRequest: header.downlinkRequest,
		UtilityMessage:  header.utilityMessage,
		Squawk:          SquawkFromIdentityCode(header.payload13),
		ICAO:            icao,
	}, nil
}
