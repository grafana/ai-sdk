package extensionhost

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/internal/streamusage"
	"github.com/grafana/ai-sdk/provider"
)

const streamBuffer = 64

const (
	deniedPartMessage      = "a gateway extension denied the response"
	unavailablePartMessage = "a gateway extension is unavailable"
)

// pipeline runs the applicable extensions' phases around one selection's
// model. The model it wraps is the runtime's selected model, so a call that a
// request phase ends never enters it.
type pipeline struct {
	provider.LanguageModel
	host       *Host
	lists      *lists
	source     extension.Source
	requested  string
	canonical  string
	candidates []extension.Candidate
	attempts   func(error) []execution.Attempt
}

// call is one call's pipeline state. Its phases run sequentially: request
// phases on the caller's goroutine, stream response phases on the forwarding
// goroutine, and observation on its own goroutine after the outcome.
type call struct {
	host          *Host
	lists         *lists
	attempts      func(error) []execution.Attempt
	views         []*extension.Call
	started       time.Time
	correlationID string
	denial        *extension.Denial
}

type verdict uint8

const (
	verdictPass verdict = iota + 1
	verdictSkipped
	verdictDenied
	verdictFailClosed
)

func (p *pipeline) newCall(ctx context.Context) *call {
	base := p.host.requestInfo(ctx)
	base.Keys, base.Source, base.RequestedModel, base.CanonicalModel = p.source.Keys(), p.source, p.requested, p.canonical
	views := make([]*extension.Call, len(p.lists.byName))
	for i := range views {
		view := base
		view.Candidates = slices.Clone(p.candidates)
		views[i] = &view
	}
	return &call{host: p.host, lists: p.lists, attempts: p.attempts, views: views, started: p.host.clock(), correlationID: base.CorrelationID}
}

// run runs one phase of the extension at position i and applies its outcome:
// a denial where the phase may deny, or a failure under the extension's
// failure policy.
func (c *call) run(i int, phase extension.Phase, mayDeny bool, step func() error) verdict {
	definition := c.lists.byName[i].definition
	err := contain(step)
	if err == nil {
		return verdictPass
	}
	if mayDeny && errors.Is(err, extension.ErrDenied) {
		c.denial = &extension.Denial{Extension: definition.Name, Phase: phase}
		c.host.recordDenial(definition, phase)
		return verdictDenied
	}
	c.host.recordFailure(definition, phase, classOf(err), c.correlationID)
	if definition.FailurePolicy == extension.FailClosed {
		return verdictFailClosed
	}
	return verdictSkipped
}

// verdictErr returns the call's error for a verdict that ends it.
func verdictErr(result verdict) error {
	switch result {
	case verdictDenied:
		return extension.ErrDenied
	case verdictFailClosed:
		return extension.ErrUnavailable
	default:
		return nil
	}
}

// request runs admit, the request transforms with the host policy check, and
// request inspection, and returns the parameters the model receives.
func (c *call) request(ctx context.Context, options provider.CallOptions) (provider.CallOptions, error) {
	for i, built := range c.lists.byName {
		if built.admitter == nil {
			continue
		}
		if err := verdictErr(c.run(i, extension.PhaseAdmit, true, func() error { return built.admitter.Admit(ctx, c.views[i]) })); err != nil {
			return provider.CallOptions{}, err
		}
	}
	params := options
	for _, i := range c.lists.transforms {
		built := c.lists.byName[i]
		var transformed provider.CallOptions
		result := c.run(i, extension.PhaseTransformRequest, false, func() error {
			var err error
			transformed, err = built.transformer.TransformRequest(ctx, c.views[i], execution.CloneOptions(params))
			if err == nil && !execution.PolicyAllows(transformed) {
				return errPolicyRefused
			}
			return err
		})
		if err := verdictErr(result); err != nil {
			return provider.CallOptions{}, err
		}
		if result == verdictPass {
			params = transformed
		}
	}
	for i, built := range c.lists.byName {
		if built.requestInspector == nil {
			continue
		}
		if err := verdictErr(c.run(i, extension.PhaseInspectRequest, true, func() error {
			return built.requestInspector.InspectRequest(ctx, c.views[i], execution.CloneOptions(params))
		})); err != nil {
			return provider.CallOptions{}, err
		}
	}
	return params, nil
}

// inspect runs response inspection on one normalized part.
func (c *call) inspect(ctx context.Context, part extension.Part) error {
	for i, built := range c.lists.byName {
		if built.responseInspector == nil {
			continue
		}
		if err := verdictErr(c.run(i, extension.PhaseInspectResponse, true, func() error {
			return built.responseInspector.InspectResponse(ctx, c.views[i], part)
		})); err != nil {
			return err
		}
	}
	return nil
}

// outcomeOf returns the outcome of a call that ended with err.
func outcomeOf(ctx context.Context, err error) extension.Outcome {
	switch {
	case errors.Is(err, extension.ErrDenied):
		return extension.OutcomeDenied
	case ctx.Err() != nil, errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return extension.OutcomeCancelled
	default:
		return extension.OutcomeError
	}
}

// finish dispatches the call's record to its observers, off the response's
// critical path. modelErr is the selected model's own outcome, which decides
// the attempt synthesized for a single candidate.
func (c *call) finish(ctx context.Context, outcome extension.Outcome, usage provider.Usage, modelErr error) {
	c.dispatch(ctx, c.reserve(), outcome, usage, modelErr)
}

// reserve reserves the call's observation, so shutdown waits for it, before
// the call's response ends. After shutdown has begun, the observation is
// dropped and counted.
func (c *call) reserve() bool {
	if !c.lists.observer {
		return false
	}
	if !c.host.observations.reserve() {
		c.host.dropped.Inc()
		return false
	}
	return true
}

// dispatch runs a reserved observation with the call's record. The record's
// attempts are read from the runtime's capture now, when the outcome is
// decided.
func (c *call) dispatch(ctx context.Context, reserved bool, outcome extension.Outcome, usage provider.Usage, modelErr error) {
	if !reserved {
		return
	}
	record := extension.Record{Outcome: outcome, DeniedBy: c.denial, Usage: usage, Attempts: physicalAttempts(c.attempts(modelErr)), Started: c.started, Duration: c.host.clock().Sub(c.started)}
	detached := context.WithoutCancel(ctx)
	c.host.observations.run(func() { c.observe(detached, record) })
}

func (c *call) observe(ctx context.Context, record extension.Record) {
	ctx, cancel := context.WithTimeout(ctx, observeTimeout)
	defer cancel()
	for i, built := range c.lists.byName {
		if built.observer == nil {
			continue
		}
		own := record
		own.Attempts = slices.Clone(record.Attempts)
		if record.DeniedBy != nil {
			denial := *record.DeniedBy
			own.DeniedBy = &denial
		}
		if err := contain(func() error { return built.observer.Observe(ctx, c.views[i], own) }); err != nil {
			c.host.recordFailure(built.definition, extension.PhaseObserve, classOf(err), c.correlationID)
		}
	}
}

// checksOutput reports whether the pipeline reads the model's output: for
// response inspection, or for the observed outcome and usage.
func (l *lists) checksOutput() bool { return l.responseInspector || l.observer }

// invoked runs the selected model. When it panics, the call's record is still
// dispatched, as an error, while the panic continues to the runtime, which
// contains it.
func invoked[T any](ctx context.Context, c *call, invoke func() (T, error)) (T, error) {
	finished := false
	defer func() {
		if !finished {
			c.finish(ctx, extension.OutcomeError, provider.Usage{}, errNoModelResult)
		}
	}()
	value, err := invoke()
	finished = true
	return value, err
}

func (p *pipeline) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	c := p.newCall(ctx)
	params, err := c.request(ctx, options)
	if err != nil {
		c.finish(ctx, outcomeOf(ctx, err), provider.Usage{}, err)
		return nil, err
	}
	result, err := invoked(ctx, c, func() (*provider.GenerateResult, error) { return p.LanguageModel.DoGenerate(ctx, params) })
	if err != nil || result == nil {
		c.finish(ctx, outcomeOf(ctx, err), provider.Usage{}, modelError(err))
		return result, err
	}
	if !p.lists.checksOutput() {
		return result, nil
	}
	history, _ := execution.RebuildToolHistory(options.Prompt)
	if execution.ValidateGenerateResult(result, history) != nil {
		c.finish(ctx, settled(ctx, extension.OutcomeInvalidOutput), result.Usage, nil)
		return result, nil
	}
	if p.lists.responseInspector {
		for _, part := range normalizeResult(result) {
			if err := c.inspect(ctx, part); err != nil {
				c.finish(ctx, outcomeOf(ctx, err), result.Usage, nil)
				return nil, err
			}
		}
	}
	c.finish(ctx, settled(ctx, extension.OutcomeSuccess), result.Usage, nil)
	return result, nil
}

// settled returns outcome, or cancelled when ctx ended first: the runtime then
// discards the result.
func settled(ctx context.Context, outcome extension.Outcome) extension.Outcome {
	if ctx.Err() != nil {
		return extension.OutcomeCancelled
	}
	return outcome
}

func (p *pipeline) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	c := p.newCall(ctx)
	params, err := c.request(ctx, options)
	if err != nil {
		c.finish(ctx, outcomeOf(ctx, err), provider.Usage{}, err)
		return nil, err
	}
	if !p.lists.checksOutput() {
		return p.LanguageModel.DoStream(ctx, params)
	}
	modelContext, cancel := context.WithCancel(ctx)
	established := false
	defer func() {
		if !established {
			cancel()
		}
	}()
	result, err := invoked(ctx, c, func() (*provider.StreamResult, error) { return p.LanguageModel.DoStream(modelContext, params) })
	if err != nil || result == nil || result.Stream == nil {
		c.finish(ctx, outcomeOf(ctx, err), provider.Usage{}, modelError(err))
		return result, err
	}
	established = true
	history, _ := execution.RebuildToolHistory(options.Prompt)
	out := make(chan provider.StreamPart, streamBuffer)
	go p.forward(ctx, cancel, c, execution.NewStreamValidator(history), result.Stream, out)
	return &provider.StreamResult{Stream: out, Request: result.Request, Response: result.Response}, nil
}

// forward inspects and delivers the model's stream parts. A response abort
// withholds the part, delivers one error part and stops the model stream. The
// record is dispatched after out is closed.
func (p *pipeline) forward(ctx context.Context, cancel context.CancelFunc, c *call, validator *execution.StreamValidator, in <-chan provider.StreamPart, out chan<- provider.StreamPart) {
	defer cancel()
	var usage streamusage.Aggregator
	outcome, invalid, closed, delivered := extension.OutcomeError, false, false, false
loop:
	for {
		select {
		case <-ctx.Done():
			outcome = extension.OutcomeCancelled
			break loop
		case part, ok := <-in:
			if !ok {
				closed = true
				if validator.SegmentFinished() {
					outcome = extension.OutcomeSuccess
				}
				break loop
			}
			usage.Observe(part)
			if !invalid && validator.Check(part) != nil {
				invalid = true
			}
			if normalized, inspected := normalizePart(part); inspected && !invalid && p.lists.responseInspector {
				if err := c.inspect(ctx, normalized); err != nil {
					outcome = outcomeOf(ctx, err)
					cancel()
					select {
					case out <- abortPart(err):
						delivered = true
					case <-ctx.Done():
					}
					break loop
				}
			}
			select {
			case out <- part:
				delivered = true
			case <-ctx.Done():
				outcome = extension.OutcomeCancelled
				break loop
			}
		}
	}
	if !closed {
		execution.StartDrain(in, p.host.drainTimeout)
	}
	if invalid && outcome != extension.OutcomeDenied {
		outcome = extension.OutcomeInvalidOutput
	}
	modelErr := streamModelError(ctx, delivered)
	reserved := c.reserve()
	close(out)
	streamUsage, _ := usage.Usage()
	c.dispatch(ctx, reserved, outcome, streamUsage, modelErr)
}

// errNoModelResult is the outcome of a selected model that ended without a
// result, or of a stream that ended without a part.
var errNoModelResult = errors.New("extensions: the model ended without a result")

// modelError returns the selected model's outcome for a call that ended with
// err and no usable result.
func modelError(err error) error {
	if err == nil {
		return errNoModelResult
	}
	return err
}

// streamModelError returns the outcome of an established stream's selected
// model, as the adapter projects it at the first part it waits for: answered
// once a part was delivered, otherwise ended by the context or without output.
// It must be read before the output is closed, because the adapter cancels ctx
// when it sees the close.
func streamModelError(ctx context.Context, delivered bool) error {
	switch {
	case delivered:
		return nil
	case ctx.Err() != nil:
		return ctx.Err()
	default:
		return errNoModelResult
	}
}

// attemptOutcomes maps the runtime's attempt outcomes to the extension
// vocabulary.
var attemptOutcomes = map[fallback.AttemptOutcome]extension.AttemptOutcome{
	fallback.AttemptSelected: extension.AttemptSelected,
	fallback.AttemptFailed:   extension.AttemptFailed,
	fallback.AttemptCanceled: extension.AttemptCanceled,
}

// physicalAttempts returns the extension view of a call's projected attempts:
// identity, outcome and status code, without failure text.
func physicalAttempts(projected []execution.Attempt) []extension.Attempt {
	if len(projected) == 0 {
		return nil
	}
	attempts := make([]extension.Attempt, len(projected))
	for i, attempt := range projected {
		attempts[i] = extension.Attempt{Provider: attempt.Provider, ProviderInstance: attempt.ProviderInstance, ProviderModel: attempt.ModelID, Outcome: attemptOutcomes[attempt.Outcome]}
		if attempt.Error != nil {
			attempts[i].StatusCode = attempt.Error.StatusCode
		}
	}
	return attempts
}

// abortPart is the error part that ends a stream an extension aborted.
func abortPart(err error) provider.StreamPart {
	message := unavailablePartMessage
	if errors.Is(err, extension.ErrDenied) {
		message = deniedPartMessage
	}
	retryable := !errors.Is(err, extension.ErrDenied)
	return provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: message, IsRetryable: &retryable, Cause: err})}
}

// normalizePart returns the inspected form of a stream part.
func normalizePart(part provider.StreamPart) (extension.Part, bool) {
	switch part.Type {
	case provider.PartTextDelta:
		return extension.Part{Type: extension.PartText, ID: part.ID, Text: part.Delta}, true
	case provider.PartReasoningDelta:
		return extension.Part{Type: extension.PartReasoning, ID: part.ID, Text: part.Delta}, true
	case provider.PartToolCall:
		return extension.Part{Type: extension.PartToolCall, ID: part.ToolCallID, ToolName: part.ToolName, Input: part.Input}, true
	case provider.PartFinish:
		return extension.Part{Type: extension.PartFinish}, true
	default:
		return extension.Part{}, false
	}
}

// normalizeResult returns the inspected parts of a unary result: its text,
// reasoning and tool calls in order, then a finish.
func normalizeResult(result *provider.GenerateResult) []extension.Part {
	parts := make([]extension.Part, 0, len(result.Content)+1)
	for _, content := range result.Content {
		switch content.Type {
		case provider.ContentText:
			parts = append(parts, extension.Part{Type: extension.PartText, ID: content.ID, Text: content.Text})
		case provider.ContentReasoning:
			parts = append(parts, extension.Part{Type: extension.PartReasoning, ID: content.ID, Text: content.Text})
		case provider.ContentToolCall:
			parts = append(parts, extension.Part{Type: extension.PartToolCall, ID: content.ToolCallID, ToolName: content.ToolName, Input: string(content.Input)})
		}
	}
	return append(parts, extension.Part{Type: extension.PartFinish})
}
