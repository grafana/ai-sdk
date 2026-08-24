# Writing middleware

Language-model middleware is the model-call customization boundary. It wraps a
`provider.LanguageModel`, so the same behavior can apply to generation, agents,
registries, fallback models, and any other consumer of that model.

Write middleware when the built-in and optional integrations do not provide a
behavior that must be consistent across model calls. Keep behavior explicit in
application code when it belongs to one workflow or request.

## Decide whether middleware is the right boundary

| You need | Prefer |
|---|---|
| Shared request policy, normalization, caching, observation, or result adaptation | Middleware |
| Settings for one model call | Call options on that call |
| One lifecycle around all model steps, tools, retries, and fallback attempts | Orchestration callbacks |
| Application-specific retrieval or actions | Application code or tools |
| HTTP authentication, routing, or SSE behavior | `net/http` middleware or stream helpers |
| Support for another model-service protocol | A provider implementation |

Middleware is useful for cross-cutting behavior, but it can also hide important
work. Keep domain decisions, consequential actions, and workflow-specific
retrieval visible unless they intentionally form a shared model policy.

## Understand the call lifecycle

Each middleware layer transforms its input, runs its matching wrapper, and then
delegates to the next layer:

```text
caller
  → first.TransformParams
  → first.WrapGenerate or first.WrapStream
    → second.TransformParams
    → second.WrapGenerate or second.WrapStream
      → provider
    ← second result or stream
  ← first result or stream
```

The first middleware is outermost. An outer wrapper runs before inner request
transforms and sees the result after inner result transforms. Place middleware
according to whether it should observe the caller's request or the request that
is closer to the provider.

Middleware runs once per model invocation, not once per application request. A
multi-step agent, retry, or fallback operation can invoke models several times.

## Choose the narrowest hook

A `middleware.Middleware` has optional hooks. Set only the hooks the behavior
needs:

| Hook | Use it for |
|---|---|
| `TransformParams` | Validate or modify prompts, tools, settings, headers, or provider options before a call |
| `WrapGenerate` | Short-circuit, observe, or transform a non-streaming provider call |
| `WrapStream` | Short-circuit, observe, replace, or transform a provider stream |
| Metadata overrides | Intentionally change the wrapper's provider, model ID, or supported URLs |

`TransformParams` receives the call type, current call options, and inner model.
Its returned options reach the wrapper hook in the same layer. Returning an
error prevents that layer from calling the inner model.

Both wrapper hooks receive `DoGenerate` and `DoStream` delegates bound to the
transformed options. Normally call the matching delegate exactly once. Calling
neither intentionally short-circuits the provider, while calling more than once
creates multiple provider calls. Cross-mode delegation is intended for adapters
such as simulated streaming, not routine middleware.

`aisdk.GenerateText` collects a `StreamText` result and therefore invokes
`LanguageModel.DoStream`; it does not invoke `DoGenerate`. `WrapGenerate` covers
consumers that call `LanguageModel.DoGenerate` directly. Reusable middleware
should support both methods when its behavior must hold for every
`LanguageModel` consumer.

## Transform request parameters

This middleware enforces a shared output-token ceiling while preserving a lower
caller-provided limit:

```go
func OutputBudget(limit int) middleware.Middleware {
	return middleware.Middleware{
		TransformParams: func(
			_ context.Context,
			input middleware.TransformParamsInput,
		) (provider.CallOptions, error) {
			params := input.Params
			if params.MaxOutputTokens == nil || *params.MaxOutputTokens > limit {
				maxOutputTokens := limit
				params.MaxOutputTokens = &maxOutputTokens
			}
			return params, nil
		},
	}
}
```

Apply it like any other middleware:

```go
model := middleware.WrapLanguageModel(baseModel, OutputBudget(1_024))
```

`provider.CallOptions` is passed by value, but its slices, maps, pointers, and
nested message content still refer to shared data. Clone any collection before
changing its elements or keys. A middleware should not mutate caller-owned
prompts, tools, headers, or provider options as a side effect.

Use `input.Type` when generate and stream calls require different parameters.
Use `input.Model` when policy depends on the identity or capabilities of the
next model in the chain.

## Wrap a generate call

A generate wrapper can run work before and after the inner call, return a cached
result without delegating, or transform selected result parts:

```go
func ObserveGenerate(
	observe func(time.Duration, error),
) middleware.Middleware {
	return middleware.Middleware{
		WrapGenerate: func(
			ctx context.Context,
			params middleware.WrapGenerateParams,
		) (*provider.GenerateResult, error) {
			started := time.Now()
			result, err := params.DoGenerate(ctx)
			observe(time.Since(started), err)
			return result, err
		},
	}
}
```

Preserve every result field that the middleware does not intentionally change,
including non-text content, usage, warnings, request and response metadata, and
provider metadata. Return inner errors unless the middleware has a documented
replacement or recovery policy.

## Wrap a stream safely

A stream wrapper returns after the inner stream opens. Measuring only the call
to `DoStream` therefore measures stream setup, not completion. Observe the
returned parts when behavior depends on streamed output.

`middleware.TransformStream` handles the output channel, normal input closure,
context cancellation, and request/response metadata. This example observes
every emitted part without changing the stream:

```go
func ObserveStreamParts(
	observe func(provider.StreamPart),
) middleware.Middleware {
	return middleware.Middleware{
		WrapStream: func(
			ctx context.Context,
			params middleware.WrapStreamParams,
		) (*provider.StreamResult, error) {
			result, err := params.DoStream(ctx)
			if err != nil {
				return nil, err
			}

			return middleware.TransformStream(
				ctx,
				result,
				func(
					part provider.StreamPart,
					emit func(provider.StreamPart),
				) {
					observe(part)
					emit(part)
				},
				nil,
			), nil
		},
	}
}
```

The transform function can emit zero, one, or many parts for each input part.
Its optional flush function runs when the input stream closes normally, but not
when the context is cancelled. `TransformStream` has no completion or
cancellation callback, so use a context-aware stream tee when instrumentation
must distinguish normal closure, error parts, and cancellation. Do not use flush
for critical cleanup or persistence.

A safe stream transform must:

- forward every unrelated part;
- preserve part IDs and start/delta/end ordering;
- preserve finish reasons, usage, errors, and provider metadata;
- handle meaningful text that is split across arbitrary chunk boundaries;
- avoid blocking the stream on slow observation or external I/O;
- stop when the request context is cancelled.

Errors returned by `DoStream` happen before the stream opens. Errors after that
point normally arrive as `provider.PartError` values and must continue through
the stream unless the middleware intentionally replaces them.

Callers should drain a returned stream whenever possible. When abandoning a
stream, cancel its context so context-aware providers and middleware can stop.
Cancellation cannot release a relay that ignores the context. Any wrapper that
starts a goroutine must make channel sends context-aware or drain its upstream
on cancellation so it cannot remain blocked publishing output.

## Preserve context and concurrency

Pass the received context to the inner delegate. A wrapper may derive a timeout
or add values before delegation, but should not replace the request context with
`context.Background()`.

A middleware value may be reused by many models and concurrent requests,
especially when attached to a provider registry. Allocate request and stream
state inside the wrapper hook. Protect intentionally shared state such as caches
or counters with concurrency-safe implementations.

The transform and flush callbacks for one `TransformStream` run serially, so
per-stream state owned by those callbacks does not need its own lock.

## Choose the wrapping boundary

Attach middleware to one model when behavior is model-specific. Attach it to a
registry when every resolved model should share the same policy:

```go
models := registry.NewProviderRegistry(
	providers,
	registry.WithLanguageModelMiddleware(OutputBudget(1_024)),
)
```

Fallback placement changes the scope:

```text
middleware(fallback(primary, backup))
    one middleware invocation around the fallback operation

fallback(middleware(primary), middleware(backup))
    one middleware invocation for each candidate attempt
```

Wrap candidates individually for candidate-level logging, metrics, policy, or
caching. Wrap the fallback model when the behavior should treat fallback as one
model operation.

Use metadata overrides sparingly. Provider and model identity affect logs,
metrics, cache keys, and model selection diagnostics. Override them only when
the wrapper intentionally exposes a different public identity.

## Package reusable middleware

For an application-local transform, a small function returning
`middleware.Middleware` is enough. A reusable package should normally expose:

```go
func Middleware(opts Options) middleware.Middleware
func Wrap(base provider.LanguageModel, opts Options) provider.LanguageModel
```

If construction can fail, return the error while building the middleware rather
than deferring configuration failures until a model call. Construct shared,
stateful middleware once and reuse it.

Keep reusable middleware model-agnostic where possible. Provider-specific
behavior should be selected explicitly through provider identity or typed
provider options, and tested against every supported provider.

## Test the contract

Use a small hand-written `provider.LanguageModel` fake and test the wrapped
model rather than only invoking hook functions directly. Cover:

- generate and stream behavior independently;
- transformed options and preservation of unrelated fields;
- delegation count and intentional short-circuiting;
- errors before stream creation and error parts during streaming;
- stream ordering, metadata, closure, and cancellation;
- middleware ordering and fallback placement;
- concurrent reuse with `go test -race`;
- metadata-only defaults for any logging or recording behavior.

See [Testing model-backed code](../guides/testing.md) for a deterministic model
example.

## Before shipping

Check that the middleware:

- does not expose prompts, reasoning, tools, credentials, or provider payloads
  without an explicit data policy;
- does not silently swallow provider or policy errors;
- preserves opaque fields and content parts it does not understand;
- applies consistently to both provider call modes where required;
- documents external I/O, latency, failure, caching, and short-circuit behavior;
- has deliberate ordering and wrapping-boundary guidance.

## Reference

- [`middleware` package](https://pkg.go.dev/github.com/grafana/ai-sdk/middleware)
- [`provider.LanguageModel`](https://pkg.go.dev/github.com/grafana/ai-sdk/provider#LanguageModel)
- [`registry.WithLanguageModelMiddleware`](https://pkg.go.dev/github.com/grafana/ai-sdk/registry#WithLanguageModelMiddleware)

---

← [Middleware overview](overview.md) · [Docs index](../README.md) · [Structured logging →](structured-logging.md)
