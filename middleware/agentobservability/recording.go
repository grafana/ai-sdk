package agentobservability

import (
	"context"
	"log"
	"sync"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

// SpanNameHooksPreflight is the OTel span name HooksMiddleware emits for each
// EvaluateHook call. It is the only span this package opens directly: the
// agento11y client owns the canonical generation span (gen_ai.* semantic
// conventions plus agento11y.generation.id) via StartGeneration /
// StartStreamingGeneration, and this middleware does not wrap or duplicate it.
const SpanNameHooksPreflight = "aisdk.hooks.preflight"

// Span attribute keys carried by the hooks preflight span. ai-sdk owns these
// keys; the agento11y SDK neither produces nor reads them.
const (
	SpanAttrHooksResult = "aisdk.hooks.result"
	SpanAttrHooksAction = "aisdk.hooks.action"
	SpanAttrHooksRuleID = "aisdk.hooks.rule_id"
)

// streamRecordingBuffer is the bounded channel capacity used when teeing the
// inner model's stream. Sized to match the buffering established providers
// use internally (e.g. the anthropic provider streams via a 64-deep channel).
const streamRecordingBuffer = 64

// tracerName is the instrumentation library name reported on every span this
// middleware opens (currently only the hooks preflight span).
const tracerName = "github.com/grafana/ai-sdk/middleware/agentobservability"

// nilContextProviderLogger ensures the "ContextProvider is nil" warning is
// emitted at most once per process for the entire middleware module, not
// per-instance. Both Recording and Hooks share the same flag.
var nilContextProviderLogger sync.Once

// RecordingMiddleware returns a middleware that records every call through
// the wrapped model as an Agent Observability generation. The Generate path records on
// success or call error; the Stream path tees the result channel, observes
// each part via a StreamRecorder, and records the assembled Generation when
// upstream closes or the request context is canceled.
//
// This middleware does NOT open its own OTel span. The agento11y client's
// StartGeneration / StartStreamingGeneration already opens the canonical
// generation span using gen_ai.* semantic conventions; wrapping that span
// would duplicate every attribute. Error states reach the trace via
// recorder.SetCallError, which agento11y stamps onto its span as
// error.type / error.category.
//
// Resolution rules:
//   - opts.ClientResolver(ctx) == nil OR opts.ClientResolver == nil → the
//     call passes through unchanged, no Generation is recorded.
//   - opts.ContextProvider == nil → a single Warn is emitted via log.Default
//     once per process; recording still proceeds using agento11y.*FromContext
//     fallbacks.
//
// The middleware never mutates the request params or result payload.
func RecordingMiddleware(opts RecordingOptions) middleware.Middleware {
	return middleware.Middleware{
		WrapGenerate: func(ctx context.Context, p middleware.WrapGenerateParams) (*provider.GenerateResult, error) {
			return wrapRecordingGenerate(ctx, opts, p)
		},
		WrapStream: func(ctx context.Context, p middleware.WrapStreamParams) (*provider.StreamResult, error) {
			return wrapRecordingStream(ctx, opts, p)
		},
	}
}

func wrapRecordingGenerate(ctx context.Context, opts RecordingOptions, p middleware.WrapGenerateParams) (*provider.GenerateResult, error) {
	// Resolve the client first so consumers without Agent Observability
	// don't trip the once-per-process "ContextProvider is nil" warning on
	// every request — the warning is only meaningful when recording would
	// otherwise proceed.
	client := resolveClient(ctx, opts.ClientResolver)
	if client == nil {
		return p.DoGenerate(ctx)
	}

	ctxInfo := resolveContextInfo(ctx, opts.ContextProvider)
	start := BuildGenerationStart(ctx, p.Model.Provider(), p.Model.ModelID(), ctxInfo)
	ctx, recorder := client.StartGeneration(ctx, start)
	defer recorder.End()

	result, err := p.DoGenerate(ctx)
	if err != nil {
		recorder.SetCallError(err)
		return nil, err
	}

	recorder.SetResult(mapGenerateResultWithStart(p.Params, result, ctxInfo, start), nil)
	return result, nil
}

func wrapRecordingStream(ctx context.Context, opts RecordingOptions, p middleware.WrapStreamParams) (*provider.StreamResult, error) {
	// See wrapRecordingGenerate for the resolve-client-first rationale.
	client := resolveClient(ctx, opts.ClientResolver)
	if client == nil {
		return p.DoStream(ctx)
	}

	ctxInfo := resolveContextInfo(ctx, opts.ContextProvider)
	start := BuildGenerationStart(ctx, p.Model.Provider(), p.Model.ModelID(), ctxInfo)
	streamCtx, recorder := client.StartStreamingGeneration(ctx, start)

	upstream, err := p.DoStream(streamCtx)
	if err != nil {
		recorder.SetCallError(err)
		recorder.End()
		return nil, err
	}

	streamRec := NewStreamRecorder(start, p.Params)
	teeCh := make(chan provider.StreamPart, streamRecordingBuffer)
	teeResult := &provider.StreamResult{
		Stream:   teeCh,
		Request:  upstream.Request,
		Response: upstream.Response,
	}

	go runStreamTee(streamCtx, upstream.Stream, teeCh, streamRec, recorder, ctxInfo)
	return teeResult, nil
}

// runStreamTee shuttles parts from the inner-model stream to the consumer
// while observing each event through the StreamRecorder. It finalizes the
// recorder (SetResult and, for stream errors or cancellation, SetCallError)
// once the upstream channel closes or the request context cancels.
func runStreamTee(
	ctx context.Context,
	upstream <-chan provider.StreamPart,
	tee chan<- provider.StreamPart,
	streamRec *StreamRecorder,
	recorder *agento11y.GenerationRecorder,
	ctxInfo ContextInfo,
) {
	defer recorder.End()
	defer close(tee)

	canceled := false
streamLoop:
	for {
		select {
		case part, ok := <-upstream:
			if !ok {
				canceled = ctx.Err() != nil
				break streamLoop
			}
			streamRec.Observe(part)

			select {
			case tee <- part:
			case <-ctx.Done():
				canceled = true
				break streamLoop
			}
		case <-ctx.Done():
			canceled = true
			break streamLoop
		}
	}

	if first := streamRec.FirstChunkAt(); !first.IsZero() {
		recorder.SetFirstTokenAt(first)
	}
	if canceled {
		recorder.SetCallError(ctx.Err())
	} else if callErr := streamRec.CallError(); callErr != nil {
		recorder.SetCallError(callErr)
	}

	gen := streamRec.Generation()
	// Carry caller-supplied identity into the final Generation in case the
	// seed got normalized away by the recorder.
	if gen.UserID == "" {
		gen.UserID = ctxInfo.UserID
	}
	if gen.AgentName == "" {
		gen.AgentName = ctxInfo.AgentName
	}
	if gen.AgentVersion == "" {
		gen.AgentVersion = ctxInfo.AgentVersion
	}
	recorder.SetResult(gen, nil)
}

// resolveClient returns the *agento11y.Client for the request, treating a
// nil resolver and a nil return value identically (both → no recording).
func resolveClient(ctx context.Context, resolver ClientResolver) *agento11y.Client {
	if resolver == nil {
		return nil
	}
	return resolver(ctx)
}

// resolveContextInfo invokes the consumer's ContextProvider for the request.
// A nil provider produces a zero ContextInfo plus a once-per-process warning
// so operators notice the missing wiring early.
func resolveContextInfo(ctx context.Context, provider ContextProvider) ContextInfo {
	if provider == nil {
		nilContextProviderLogger.Do(func() {
			log.Println("agentobservability: ContextProvider is nil; generation rows will be missing per-request metadata. " +
				"Configure WrapOptions.ContextProvider to populate tenant_id, user_id, and tags.")
		})
		return ContextInfo{}
	}
	return provider(ctx)
}

// resetNilContextProviderLoggerForTest re-arms the once-per-process warning.
// Used by tests; not part of the public API.
func resetNilContextProviderLoggerForTest() {
	nilContextProviderLogger = sync.Once{}
}
