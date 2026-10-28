package execution

import (
	"errors"
	"time"
)

// Limits bounds stream liveness. Selection and the model call end at the
// request context's deadline, which the host sets.
type Limits struct {
	// StreamIdleDuration is the maximum time between represented stream parts.
	StreamIdleDuration time.Duration
	// StreamDrainDuration bounds the drain of a stream after it is closed.
	StreamDrainDuration time.Duration
}

// DefaultLimits are the limits the Gateway serves with.
var DefaultLimits = Limits{
	StreamIdleDuration:  30 * time.Second,
	StreamDrainDuration: time.Second,
}

var (
	errIdleDuration  = errors.New("execution: stream idle duration must be positive")
	errDrainDuration = errors.New("execution: stream drain duration must be positive")
)

// validate reports whether both durations are positive.
func (limits Limits) validate() error {
	if limits.StreamIdleDuration <= 0 {
		return errIdleDuration
	}
	if limits.StreamDrainDuration <= 0 {
		return errDrainDuration
	}
	return nil
}
