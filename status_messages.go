package modes

// Aircraft status / operational messages (TC 28, 29, 31)
// ======================================================
//
// These ME formats carry status, intent, and operational
// metadata orthogonal to the position / velocity / identity
// payloads. They are subtype-rich and individually well
// documented in DO-260B; this commit provides:
//
//	TC 28  Aircraft Status — full subtype 1 (Emergency / Priority
//	       Status), structural-only for subtype 2 (TCAS RA).
//	TC 29  Target State and Status — structural recognition
//	       (Subtype + raw payload); DO-260B subtype 1 sub-decoding
//	       lands in a follow-up.
//	TC 31  Aircraft Operational Status — structural recognition
//	       (Subtype + raw payload); the airborne / surface
//	       sub-decoders land in follow-ups once an empirical
//	       reference frame is available.
//
// "Structural recognition" means the dispatcher routes the frame
// to the correct typed message and the consumer sees a non-nil
// Message of the right type with the raw 7-byte ME payload —
// enough to log + count + type-switch on without the per-subtype
// fields populated.

// AircraftStatusSubtype distinguishes the TC 28 sub-formats.
type AircraftStatusSubtype uint8

// Documented TC 28 subtypes per §A.2.3.6.6.1.
const (
	AircraftStatusSubtypeReserved               AircraftStatusSubtype = 0
	AircraftStatusSubtypeEmergency              AircraftStatusSubtype = 1
	AircraftStatusSubtypeTCASResolutionAdvisory AircraftStatusSubtype = 2
)

// EmergencyState encodes the 3-bit emergency / priority state in
// TC 28 subtype 1 per DO-260B table A-13.
type EmergencyState uint8

// Documented emergency states.
const (
	EmergencyStateNone                 EmergencyState = 0
	EmergencyStateGeneral              EmergencyState = 1
	EmergencyStateLifeguard            EmergencyState = 2
	EmergencyStateMinimumFuel          EmergencyState = 3
	EmergencyStateNoCommunications     EmergencyState = 4
	EmergencyStateUnlawfulInterference EmergencyState = 5
	EmergencyStateDowned               EmergencyState = 6
	EmergencyStateReserved7            EmergencyState = 7
)

// AircraftStatusMessage is the decoded payload of a TC 28 frame.
// Subtype 1 populates EmergencyState + Squawk; subtype 2 surfaces
// the raw RA bytes for downstream RA-specific decoding.
type AircraftStatusMessage struct {
	Subtype        AircraftStatusSubtype
	EmergencyState EmergencyState
	Squawk         Squawk

	// RawRAPayload holds the unstructured payload for subtype 2
	// (TCAS RA broadcast) and for any subtype the decoder
	// doesn't yet split.
	RawRAPayload [7]byte
}

func (AircraftStatusMessage) isModesMessage() {}

// TargetStateMessage is the decoded payload of a TC 29 frame.
// Subtype 1 (DO-260B) carries selected altitude, baro setting,
// selected heading, and a handful of MCP / FCU mode bits. Until
// the full sub-decoder lands, the consumer gets the structural
// fields plus the raw 7-byte ME payload.
type TargetStateMessage struct {
	Subtype uint8
	Raw     [7]byte
}

func (TargetStateMessage) isModesMessage() {}

// OperationalStatusMessage is the decoded payload of a TC 31
// frame. Subtype 0 = airborne, subtype 1 = surface; both carry
// capability + operational-mode bits the consumer needs for
// link-version and capability negotiation. Structural-only
// today; per-subtype sub-decoding lands in a follow-up.
type OperationalStatusMessage struct {
	Subtype uint8
	Raw     [7]byte
}

func (OperationalStatusMessage) isModesMessage() {}

// decodeAircraftStatus parses a TC 28 ME payload. Subtype 1
// gets full decoding; other subtypes (TCAS RA broadcast,
// reserved values) return a structural message with
// RawRAPayload populated.
func decodeAircraftStatus(mePayload []byte) AircraftStatusMessage {
	const (
		subtypeMask     byte   = 0x07
		emergMask       byte   = 0xE0
		emergShift      byte   = 5
		idCodeMask      uint16 = 0x1FFF
		idCodeHighShift uint   = 8
		idCodeHighMask  byte   = 0x1F
	)

	subtype := AircraftStatusSubtype(mePayload[0] & subtypeMask)
	out := AircraftStatusMessage{Subtype: subtype}

	if subtype == AircraftStatusSubtypeEmergency {
		out.EmergencyState = EmergencyState((mePayload[1] & emergMask) >> emergShift)

		identityCode := uint16(mePayload[1]&idCodeHighMask)<<idCodeHighShift | uint16(mePayload[2])
		identityCode &= idCodeMask
		out.Squawk = SquawkFromIdentityCode(identityCode)

		return out
	}

	copy(out.RawRAPayload[:], mePayload)

	return out
}

// decodeTargetState extracts the subtype and stores the raw
// ME payload. Per-subtype expansion is a follow-up.
func decodeTargetState(mePayload []byte) TargetStateMessage {
	const subtypeMask byte = 0x07

	out := TargetStateMessage{Subtype: mePayload[0] & subtypeMask}
	copy(out.Raw[:], mePayload)

	return out
}

// decodeOperationalStatus extracts the subtype and stores the
// raw ME payload. Per-subtype expansion is a follow-up.
func decodeOperationalStatus(mePayload []byte) OperationalStatusMessage {
	const subtypeMask byte = 0x07

	out := OperationalStatusMessage{Subtype: mePayload[0] & subtypeMask}
	copy(out.Raw[:], mePayload)

	return out
}
