package modes

import "testing"

func TestDecodeAircraftStatusEmergency(t *testing.T) {
	t.Parallel()

	const (
		subtype  = 1 // emergency
		emergSet = uint8(EmergencyStateUnlawfulInterference)
	)

	// Pack a synthetic ME payload: TC=28, subtype=1, emergency=5,
	// ID code = squawk 7500 (matches "hijack" + the unlawful-
	// interference emergency state — the canonical pairing).
	identityCode := makeIdentityCode(7500)

	var mePayload [7]byte

	mePayload[0] = (28 << 3) | subtype
	mePayload[1] = (emergSet << 5) | byte(identityCode>>8&0x1F)
	mePayload[2] = byte(identityCode & 0xFF)

	msg := decodeAircraftStatus(mePayload[:])

	if msg.Subtype != AircraftStatusSubtypeEmergency {
		t.Errorf("Subtype = %d, want %d", msg.Subtype, AircraftStatusSubtypeEmergency)
	}

	if msg.EmergencyState != EmergencyStateUnlawfulInterference {
		t.Errorf("EmergencyState = %d, want %d (Unlawful Interference)",
			msg.EmergencyState, EmergencyStateUnlawfulInterference)
	}

	if msg.Squawk != 7500 {
		t.Errorf("Squawk = %04d, want 7500", msg.Squawk)
	}
}

func TestDecodeAircraftStatusTCASRACopiesRaw(t *testing.T) {
	t.Parallel()

	mePayload := [7]byte{(28 << 3) | 2, 0xAB, 0xCD, 0xEF, 0x12, 0x34, 0x56}

	msg := decodeAircraftStatus(mePayload[:])
	if msg.Subtype != AircraftStatusSubtypeTCASResolutionAdvisory {
		t.Errorf("Subtype = %d, want %d", msg.Subtype, AircraftStatusSubtypeTCASResolutionAdvisory)
	}

	if msg.RawRAPayload != mePayload {
		t.Errorf("RawRAPayload = %x, want %x", msg.RawRAPayload, mePayload)
	}
}

func TestDecodeTargetStateStructural(t *testing.T) {
	t.Parallel()

	mePayload := [7]byte{(29 << 3) | 1, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}

	msg := decodeTargetState(mePayload[:])
	if msg.Subtype != 1 {
		t.Errorf("Subtype = %d, want 1", msg.Subtype)
	}

	if msg.Raw != mePayload {
		t.Errorf("Raw = %x, want %x", msg.Raw, mePayload)
	}
}

func TestDecodeOperationalStatusStructural(t *testing.T) {
	t.Parallel()

	mePayload := [7]byte{31 << 3, 0x10, 0x20, 0x30, 0x40, 0x50, 0x60}

	msg := decodeOperationalStatus(mePayload[:])
	if msg.Subtype != 0 {
		t.Errorf("Subtype = %d, want 0 (airborne)", msg.Subtype)
	}

	if msg.Raw != mePayload {
		t.Errorf("Raw = %x, want %x", msg.Raw, mePayload)
	}
}
