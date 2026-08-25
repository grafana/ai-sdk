# Writing middleware

Write middleware when you need the same model-call behavior across models or
call sites. Middleware wraps `provider.LanguageModel`, so your customization
works anywhere the wrapped model is used.

## Common use cases

- apply default instructions, settings, headers, or provider options;
- enforce token budgets, allowlists, rate limits, or call policy;
- enrich prompts with retrieved context or request metadata;
- cache calls or record logs, metrics, traces, usage, or audit events;
- normalize or transform text, reasoning, JSON, tools, streams, or call modes.

## Choose a hook

Start with the narrowest hook that supports your behavior:

| Hook | Use it to |
|---|---|
| `TransformParams` | Validate or change prompts, tools, settings, headers, or provider options |
| `WrapGenerate` | Short-circuit, observe, cache, or transform a `DoGenerate` call |
| `WrapStream` | Short-circuit, observe, cache, or transform a `DoStream` call |
| Metadata overrides | Change the wrapped model's provider, model ID, or supported URLs |

All hooks are optional. `TransformParams` runs before the matching wrapper and
can return an error before the model is called. Wrappers receive delegates for
both call modes, already bound to the transformed parameters.

Normally call the matching delegate exactly once. Calling neither short-circuits
the provider; calling it more than once creates multiple model calls. Use the
other delegate only for an intentional cross-mode adapter.

`aisdk.GenerateText` collects a streaming result, so it invokes `DoStream`.
Implement both wrappers if your middleware must also support consumers that call
`DoGenerate` directly.

## Write a parameter transform

A middleware is a value with the hooks you need. This example enforces an output
limit while preserving a lower caller-provided value:

```go
func OutputBudget(limit int) middleware.Middleware {
	return middleware.Middleware{
		TransformParams: func(_ context.Context, input middleware.TransformParamsInput) (provider.CallOptions, error) {
			params := input.Params
			if params.MaxOutputTokens == nil || *params.MaxOutputTokens > limit {
				value := limit
				params.MaxOutputTokens = &value
			}
			return params, nil
		},
	}
}
```

`CallOptions` is copied by value, but its slices, maps, pointers, and nested
message content still share storage. Clone every collection level you modify so
you do not mutate caller-owned data.

Use `input.Type` when generate and stream calls need different parameters. Use
`input.Model` when behavior depends on the next model in the chain.

## Wrap results and streams

In `WrapGenerate`, call `params.DoGenerate(ctx)`, handle its error, then inspect
or transform the result. Preserve every field you do not own, including non-text
content, usage, warnings, and request, response, and provider metadata.

In `WrapStream`, call `params.DoStream(ctx)` first, then use
`middleware.TransformStream` to inspect or replace parts:

```go
WrapStream: func(ctx context.Context, params middleware.WrapStreamParams) (*provider.StreamResult, error) {
	result, err := params.DoStream(ctx)
	if err != nil {
		return nil, err
	}
	return middleware.TransformStream(ctx, result,
		func(part provider.StreamPart, emit func(provider.StreamPart)) {
			observe(part)
			emit(part)
		},
		nil,
	), nil
},
```

A transform may emit zero, one, or many parts. Preserve unrelated parts, IDs,
ordering, finish reasons, usage, errors, and provider metadata. Buffer when a
transformation can span chunk boundaries. Keep observation callbacks fast:
slow I/O blocks upstream reads.

`DoStream` errors happen before the stream opens. Later errors normally arrive
as `provider.PartError` values and should remain in the stream. The optional
flush callback runs only when the input closes normally, not on cancellation.

Pass the received context to delegates. Callers should drain streams. When they
abandon one, cancellation only stops context-aware components. Any goroutine you
create must make sends context-aware or drain upstream on cancellation.

## Wrap and compose the model

Apply your middleware with `WrapLanguageModel`:

```go
model := middleware.WrapLanguageModel(
	baseModel,
	OutputBudget(1_024),
	otherMiddleware,
)
```

The first middleware is outermost:

```text
request  → first → second → provider
response ← first ← second ← provider
```

Order determines whether your middleware sees caller parameters or parameters
transformed closer to the provider. Keep per-call state inside hooks and protect
shared caches or counters because middleware may serve concurrent requests.

Attach middleware to a registry when every resolved model needs it. With
fallback, wrap each candidate for candidate-level behavior or wrap the fallback
model to treat fallback as one model operation.

## Test before reuse

Treat prompts, reasoning, tools, credentials, and provider payloads as sensitive.
Make capture opt-in, bounded, and redacted.

Test the wrapped model with a hand-written `provider.LanguageModel` fake. Cover
both call modes, transformed and untouched fields, delegate count, errors,
stream order and cancellation, composition, and concurrent use with
`go test -race`.

If you publish the middleware, expose a constructor returning
`middleware.Middleware` and optionally a `Wrap` helper. Validate configuration
at construction and document ordering, external I/O, short-circuiting,
sensitive-data handling, and supported providers.

See [Testing model-backed code](../guides/testing.md) for a deterministic model
example.

## Reference

- [`middleware` package](https://pkg.go.dev/github.com/grafana/ai-sdk/middleware)
- [`provider.LanguageModel`](https://pkg.go.dev/github.com/grafana/ai-sdk/provider#LanguageModel)

---

← [Middleware overview](overview.md) · [Docs index](../README.md) · [Structured logging →](structured-logging.md)
