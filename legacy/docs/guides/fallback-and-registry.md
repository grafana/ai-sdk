# Fallback and registry

Use fallback when a model call should try an ordered backup after an eligible
failure. Use a registry when configuration or a server request selects a model
with an ID such as `bedrock:model-id`.

These capabilities solve independent problems:

- [Fallback](#build-an-ordered-fallback) keeps calls available when a preferred
  model fails.
- [Provider registries](#resolve-provider-oriented-model-ids) construct a model
  selected by a provider-oriented ID.
- [Custom providers](#expose-a-restricted-model-namespace) expose a small
  application-controlled set of model names.
- [Gateway catalogs](#add-a-gateway-catalog-for-discovery) add public aliases,
  metadata, listing, and request-aware visibility.

A dynamically selected model can itself be a fallback when the application
needs both runtime selection and failover.

## Build an ordered fallback

Pass candidates to `fallback.New` in priority order:

```go
model, err := fallback.New(
	primary,
	secondary,
	emergency,
)
if err != nil {
	return err
}

result, err := aisdk.GenerateText(ctx, model, opts...)
```

Every model call starts with `primary`. The fallback forwards the same messages,
tools, provider options, and output requirements to each candidate until one
succeeds.

`fallback.New` accepts any positive number of initialized candidates. One
candidate is valid and provides no failover. An empty list returns
`fallback.ErrNoCandidates`.

All candidates should support the call's required input types, tools, structured
output, and context size. Candidate changes can affect quality, latency, cost,
and provider-specific metadata.

## Understand candidate selection

The default policy moves to the next candidate for:

- retryable `provider.APICallError` values;
- errors that are not `provider.APICallError` values.

The chain stops for:

- non-retryable API errors;
- status-400 API errors with a recognized context-window signal;
- cancellation or expiry of the request context.

A candidate-returned `context.DeadlineExceeded` can advance to the next candidate
when the request context remains active because the default policy treats it as
a non-API error.

At the fallback model boundary, an exhausted chain returns the last candidate
error. `StreamText` and `GenerateText` can retry the complete chain and return a
`RetryError` after retry exhaustion.

A custom decider can apply application-specific eligibility rules:

```go
model.WithDecider(func(err error) bool {
	return shouldTryNextCandidate(err)
})
```

Each call and each tool-loop step begins at the first candidate. A secondary
that serves one step does not become the first candidate for later steps.

## Know the streaming commitment point

A streaming call can move to the next candidate when `DoStream` returns an error
or its first stream part is an error. The fallback commits to a candidate after
its first non-error provider part, including bookkeeping events such as
`stream-start`. Commitment can therefore happen before client-visible content.

Errors that arrive after commitment are returned through that stream. The
fallback does not start another candidate for that call.

This boundary prevents one response from combining output produced by different
models.

## Combine retries and fallback deliberately

Provider clients, SDK orchestration, and fallback can each create additional
attempts:

1. A provider client may retry one candidate internally.
2. Fallback may advance through the candidate list.
3. If the full chain returns a retryable error, SDK retry policy may start the
   chain again from the first candidate.

Set retry and fallback limits from the request's latency and cost budget. Make
side-effecting tools idempotent and monitor provider attempts per application
request. See [Retry and timeout](retry-and-timeout.md).

## Account for usage

Each `StepResult.Usage` reports usage from the candidate that served that step.
The top-level `Usage` and `TotalUsage` both sum input-token and output-token
totals across completed steps. Cache, text, reasoning, and raw usage breakdowns
remain available on each step and are not aggregated.

Failed candidate usage is not added to any of these values. A provider may bill
work performed before an error even when it returns no usage data, so the
reported total can be lower than provider billing for the complete request.

Attach logging, metrics, or observability middleware to each candidate before
calling `fallback.New` to record candidate-level attempts. Error responses may
still omit token usage.

## Identify the serving provider

Successful provider response metadata passes through the fallback unchanged.
For a single-step `GenerateText` call, inspect:

```go
fmt.Println(result.Response.Provider)
fmt.Println(result.Response.ModelID)
```

A multi-step request can use a different candidate for each step. Inspect every
step after completion:

```go
for _, step := range result.Steps {
	fmt.Println(step.Response.Provider, step.Response.ModelID)
}
```

`StreamTextResult.Response()` blocks until orchestration completes and returns
the last step's response. `Steps()`, `OnStepFinish`, and `StreamFinishStep`
provide per-step response metadata.

The fallback model's own `Provider()` and `ModelID()` methods identify the first
candidate. Step-start model metadata also identifies that first candidate.

Serving identity is reliable only when the provider emits response metadata. If
it does not, the response provider is empty and the model ID defaults to the
fallback model's first candidate. Candidates that share the same response
identity also cannot be distinguished from the final result. Candidate-specific
middleware provides attempt-level identification in these cases.

The fallback result exposes no selected-candidate index, attempt list, or
`UsedFallback` flag.

## Resolve provider-oriented model IDs

A `ProviderRegistry` routes IDs such as `bedrock:model-id` to a registered
provider factory:

```go
models := registry.NewProviderRegistry(map[string]registry.Provider{
	"bedrock": bedrock.NewProvider(bedrock.WithRegion("us-east-1")),
})

model, err := models.LanguageModel(
	"bedrock:us.anthropic.claude-haiku-4-5-20251001-v1:0",
)
```

Use a registry when server-side configuration or requests already carry a
provider/model construction ID. Registry middleware can wrap every resolved
model in one place.

## Expose a restricted model namespace

`NewCustomProvider` maps application-visible names to constructed models:

```go
models := registry.NewCustomProvider(
	registry.WithLanguageModels(map[string]provider.LanguageModel{
		"fast":    fastModel,
		"quality": qualityModel,
	}),
)

model, err := models.LanguageModel("quality")
```

A custom provider with no fallback provider rejects unlisted IDs, creating a
model allowlist. Apply the same authorization policy to model listing and
resolution.

`registry.WithFallbackProvider` delegates unresolved model IDs to another
provider. `fallback.Model` handles failures that occur while calling an already
resolved model.

## Add a gateway catalog for discovery

Use `gateway/catalog` when clients need canonical public IDs, aliases, discovery
metadata, listing, or request-aware visibility. A registry constructs models. A
catalog controls which model names clients can discover and use.

See [Gateway model catalog](gateway-model-catalog.md).

## Reference

- [`fallback` package](https://pkg.go.dev/github.com/grafana/ai-sdk/fallback)
- [`registry` package](https://pkg.go.dev/github.com/grafana/ai-sdk/registry)

---

← [Retry and timeout](retry-and-timeout.md) · [Docs index](../README.md) · [Middleware →](../middleware/overview.md)
