package execution

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

// Config configures a [Runtime].
type Config struct {
	Selector RequestSelector
	Limits   Limits
	// ConfiguredAPIKeys are provider and exporter credentials redacted from native
	// failure summaries.
	ConfiguredAPIKeys []string `json:"-"`
}

// Runtime selects and invokes models for one adapter. It is immutable and safe
// for concurrent use.
type Runtime struct {
	selector RequestSelector
	limits   Limits
	redactor SummaryRedactor
}

var (
	errNilSelector = errors.New("execution: selector is nil")
	errInternal    = errors.New("execution: internal failure")
)

// New constructs a runtime.
func New(config Config) (*Runtime, error) {
	if config.Selector == nil {
		return nil, errNilSelector
	}
	if err := config.Limits.validate(); err != nil {
		return nil, err
	}
	return &Runtime{
		selector: config.Selector,
		limits:   config.Limits,
		redactor: NewSummaryRedactor(config.ConfiguredAPIKeys),
	}, nil
}

// Call is one selected model call. Its methods must be called from one
// goroutine.
type Call struct {
	ctx      context.Context
	model    provider.LanguageModel
	options  provider.CallOptions
	history  ToolHistory
	attempts *attemptCapture
	limits   Limits
}

// Call rebuilds the prompt's tool history, applies host request policy and
// selects the model for a request. It fails inconsistent history with
// [ErrInvalidToolHistory] before anything else. It then refuses protected call
// headers with [ErrProtectedCallHeader], and removes the call-level
// [ControlNamespace] from options to pass it to the selector. A
// context that has ended before or during selection decides the outcome, and
// its error is returned. A translator set with [WithTranslator] reaches every
// physical attempt of this call only.
func (r *Runtime) Call(ctx context.Context, modelID string, options provider.CallOptions, callOptions ...CallOption) (*Call, error) {
	var config callConfig
	for _, apply := range callOptions {
		apply(&config)
	}
	history, err := RebuildToolHistory(options.Prompt)
	if err != nil {
		return nil, err
	}
	options, gateway, err := applyPolicy(options)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	selected, err := r.selectModel(ctx, modelID, options, gateway)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	attempts := newAttemptCapture(modelID, selected, r.redactor)
	return &Call{ctx: withTranslator(attempts.withObserver(ctx), config.translator), model: selected.Model, options: options, history: history, attempts: attempts, limits: r.limits}, nil
}

// ToolHistory returns a copy of the call's pending provider-executed tool
// calls, rebuilt from its prompt.
func (c *Call) ToolHistory() ToolHistory { return c.history.clone() }

func (r *Runtime) selectModel(ctx context.Context, modelID string, options provider.CallOptions, gateway json.RawMessage) (selected Selection, err error) {
	defer func() {
		if recover() != nil {
			selected, err = Selection{}, errInternal
		}
	}()
	selected, err = r.selector(ctx, modelID, options, gateway)
	if err == nil && !validSelection(selected) {
		return Selection{}, errInternal
	}
	return selected, err
}

func validSelection(selected Selection) (valid bool) {
	defer func() {
		if recover() != nil {
			valid = false
		}
	}()
	return selected.ID != "" && utf8.ValidString(selected.ID) && !isNilInterface(selected.Model) && selected.Model.SpecificationVersion() == "v4"
}

// Generate invokes the model once. A panic or a missing result fails as
// internal, and a result returned after the context ended is discarded in
// favor of the context's error.
func (c *Call) Generate() (*provider.GenerateResult, error) {
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	modelContext, cancel := context.WithCancel(c.ctx)
	defer cancel()
	result, err := c.generate(modelContext)
	if ctxErr := c.ctx.Err(); ctxErr != nil {
		c.attempts.discard()
		return nil, ctxErr
	}
	return result, err
}

func (c *Call) generate(ctx context.Context) (result *provider.GenerateResult, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, errInternal
		}
		if result == nil && err == nil {
			err = errInternal
		}
	}()
	c.attempts.enter()
	return c.model.DoGenerate(ctx, c.options)
}

// Stream invokes the model's stream once. A failed setup, or a stream returned
// after the context ended, is canceled and drained once.
func (c *Call) Stream() (*Stream, error) {
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	modelContext, cancel := context.WithCancel(c.ctx)
	result, err := c.stream(modelContext)
	if ctxErr := c.ctx.Err(); ctxErr != nil {
		c.abandon(cancel, result)
		c.attempts.discard()
		return nil, ctxErr
	}
	if !isNilInterface(err) {
		c.abandon(cancel, result)
		return nil, err
	}
	if result == nil || result.Stream == nil {
		c.abandon(cancel, result)
		return nil, errInternal
	}
	return &Stream{
		ctx:    c.ctx,
		model:  modelContext,
		cancel: cancel,
		parts:  result.Stream,
		idle:   time.NewTimer(c.limits.StreamIdleDuration),
		limits: c.limits,
	}, nil
}

func (c *Call) stream(ctx context.Context) (result *provider.StreamResult, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, errInternal
		}
	}()
	c.attempts.enter()
	return c.model.DoStream(ctx, c.options)
}

func (c *Call) abandon(cancel context.CancelFunc, result *provider.StreamResult) {
	cancel()
	if result != nil {
		startDrain(result.Stream, c.limits.StreamDrainDuration)
	}
}

// View seals the call's attempt capture and projects it for an outcome that
// failed with err, or succeeded when err is nil.
func (c *Call) View(err error) View { return c.attempts.view(err) }

// Close seals the call's attempt capture.
func (c *Call) Close() { c.attempts.discard() }

// Wait is the outcome of waiting for a stream part.
type Wait uint8

const (
	WaitPart Wait = iota + 1
	WaitClosed
	WaitCanceled
	WaitDeadline
	WaitIdleTimeout
)

// Err returns the error a terminal wait outcome stands for, and nil for WaitPart.
func (w Wait) Err() error {
	switch w {
	case WaitPart:
		return nil
	case WaitCanceled:
		return context.Canceled
	case WaitDeadline, WaitIdleTimeout:
		return context.DeadlineExceeded
	default:
		return errInternal
	}
}

// Stream is an established model stream. Its methods must be called from one
// goroutine, and Close must be called once the adapter is done.
type Stream struct {
	ctx      context.Context
	model    context.Context
	cancel   context.CancelFunc
	parts    <-chan provider.StreamPart
	idle     *time.Timer
	limits   Limits
	waited   bool
	closeAll sync.Once
}

// Next waits for the next part. The idle duration restarts on each call after
// the first, so it measures the time since the previous part was handled.
// Every outcome other than [WaitPart] cancels the model context first.
func (s *Stream) Next() (provider.StreamPart, Wait) {
	if s.waited {
		s.idle.Reset(s.limits.StreamIdleDuration)
	}
	s.waited = true
	select {
	case part, ok := <-s.parts:
		if ok {
			return part, WaitPart
		}
		s.cancel()
		return provider.StreamPart{}, WaitClosed
	case <-s.model.Done():
		s.cancel()
		if errors.Is(s.ctx.Err(), context.DeadlineExceeded) {
			return provider.StreamPart{}, WaitDeadline
		}
		return provider.StreamPart{}, WaitCanceled
	case <-s.idle.C:
		s.cancel()
		return provider.StreamPart{}, WaitIdleTimeout
	}
}

// Close cancels the model context and starts one drain, bounded by the drain
// duration. Later calls have no effect.
func (s *Stream) Close() {
	s.closeAll.Do(func() {
		s.cancel()
		s.idle.Stop()
		startDrain(s.parts, s.limits.StreamDrainDuration)
	})
}

func startDrain(parts <-chan provider.StreamPart, duration time.Duration) {
	if parts != nil {
		go drain(parts, duration)
	}
}

func drain(parts <-chan provider.StreamPart, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-parts:
			if !ok {
				return
			}
		case <-timer.C:
			return
		}
	}
}
