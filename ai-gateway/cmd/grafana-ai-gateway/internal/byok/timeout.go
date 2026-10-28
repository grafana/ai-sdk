package byok

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptrace"
	"sync/atomic"
	"time"

	"github.com/grafana/ai-sdk/provider"
)

// ErrProviderStartTimeout reports an attempt whose provider sent no response
// byte within its providerTimeouts.byok bound. It is a deadline error, so it is
// classified as a timeout, and fallback treats it as eligible.
var ErrProviderStartTimeout = fmt.Errorf("gateway byok: provider did not start responding before its timeout: %w", context.DeadlineExceeded)

var errStartTimeoutCause = errors.New("gateway byok: provider start timeout")

// startTimeoutModel cancels an attempt whose provider has not sent its first
// response byte within timeout. After the first byte it has no effect.
type startTimeoutModel struct {
	provider.LanguageModel
	timeout time.Duration
}

func withStartTimeout(model provider.LanguageModel, timeout time.Duration) provider.LanguageModel {
	if timeout <= 0 {
		return model
	}
	return startTimeoutModel{LanguageModel: model, timeout: timeout}
}

func (m startTimeoutModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	attempt, cancel, stop := m.start(ctx)
	defer cancel(nil)
	result, err := m.LanguageModel.DoGenerate(attempt, options)
	if stop() && err != nil && ctx.Err() == nil {
		return nil, ErrProviderStartTimeout
	}
	return result, err
}

func (m startTimeoutModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	attempt, cancel, stop := m.start(ctx)
	result, err := m.LanguageModel.DoStream(attempt, options)
	if err != nil {
		timedOut := stop()
		cancel(nil)
		if timedOut && ctx.Err() == nil {
			return nil, ErrProviderStartTimeout
		}
	}
	return result, err
}

// start returns the attempt context, its cancel function and a stop function
// that disarms the timer and reports whether it had fired. The first response
// byte disarms the timer. The stream's context stays live until ctx ends.
func (m startTimeoutModel) start(ctx context.Context) (context.Context, context.CancelCauseFunc, func() bool) {
	attempt, cancel := context.WithCancelCause(ctx)
	var fired atomic.Bool
	timer := time.AfterFunc(m.timeout, func() {
		fired.Store(true)
		cancel(errStartTimeoutCause)
	})
	attempt = httptrace.WithClientTrace(attempt, &httptrace.ClientTrace{GotFirstResponseByte: func() { timer.Stop() }})
	return attempt, cancel, func() bool {
		timer.Stop()
		return fired.Load()
	}
}
