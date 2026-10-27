package execution

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLimitsValidate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limits Limits
		err    error
	}{
		{name: "defaults", limits: DefaultLimits},
		{name: "idle zero", limits: Limits{StreamDrainDuration: time.Second}, err: errIdleDuration},
		{name: "idle negative", limits: Limits{StreamIdleDuration: -time.Second, StreamDrainDuration: time.Second}, err: errIdleDuration},
		{name: "drain zero", limits: Limits{StreamIdleDuration: time.Second}, err: errDrainDuration},
		{name: "drain negative", limits: Limits{StreamIdleDuration: time.Second, StreamDrainDuration: -time.Second}, err: errDrainDuration},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.ErrorIs(t, tc.limits.validate(), tc.err)
		})
	}
}

func TestDefaultLimits(t *testing.T) {
	assert.Equal(t, Limits{StreamIdleDuration: 30 * time.Second, StreamDrainDuration: time.Second}, DefaultLimits)
}
