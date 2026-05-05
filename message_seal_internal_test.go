package modes

import "testing"

// TestMessageSealMethods exercises every concrete type's
// isModesMessage() satisfier. The interface is sealed so callers
// only type-switch — these methods are otherwise unreachable from
// tests. Calling each one directly keeps coverage honest without
// changing the public API.
func TestMessageSealMethods(t *testing.T) {
	t.Parallel()

	IdentificationMessage{}.isModesMessage()
	AirbornePositionMessage{}.isModesMessage()
	AirborneVelocityMessage{}.isModesMessage()
	SurfacePositionMessage{}.isModesMessage()
	AircraftStatusMessage{}.isModesMessage()
	TargetStateMessage{}.isModesMessage()
	OperationalStatusMessage{}.isModesMessage()
}
