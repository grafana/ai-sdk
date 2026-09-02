package v4

import "time"

type protocolClock interface {
	Now() time.Time
	NewTimer(time.Duration) protocolTimer
}

type protocolTimer interface {
	C() <-chan time.Time
	Stop() bool
}

type realProtocolClock struct{}

func (realProtocolClock) Now() time.Time { return time.Now() }

func (realProtocolClock) NewTimer(duration time.Duration) protocolTimer {
	if duration < 0 {
		duration = 0
	}
	return realProtocolTimer{timer: time.NewTimer(duration)}
}

type realProtocolTimer struct {
	timer *time.Timer
}

func (t realProtocolTimer) C() <-chan time.Time { return t.timer.C }
func (t realProtocolTimer) Stop() bool          { return t.timer.Stop() }
