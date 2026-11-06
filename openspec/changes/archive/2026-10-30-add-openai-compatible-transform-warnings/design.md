## Context

`buildRequest` already creates a body and warning slice for each invocation, then applies `WithRequestTransform`. Both HTTP paths serialize that body once for transport and request metadata. Generation returns warnings directly; streaming emits them in `PartStreamStart`, which core orchestration already collects.

The canonical reference is `test/conformance/upstream.yaml`: Vercel AI commit `b7f6255acaae7724adc0fd37aac3dae3f3466578`, `@ai-sdk/openai-compatible` 3.0.71. At that revision, `packages/openai-compatible/src/chat/openai-compatible-chat-language-model.ts` passes the `getArgs` warnings into `transformRequestBody` in both `doGenerate` and `doStream`. Tests in its `transformRequestBody` describe block verify that the callback receives warnings and that its body is sent. This is provider-adapter coverage (`mixed` in `PARITY.md`), demonstrated here with synthetic HTTP tests, not provider recordings.

## Goals / Non-Goals

Expose upstream's warning capability using Go slice and error returns, without breaking existing callers. Preserve per-call isolation, transport rejection, and request metadata. No backend policy or unrelated provider extension is included.

## Decisions

- Add `WithRequestTransformAndWarnings(func(map[string]any, []provider.Warning) (map[string]any, []provider.Warning, error))`. The callback receives existing warnings and returns the complete desired slice; it normally appends to the input. Returning the slice is necessary because Go `append` may allocate and changes the slice length. This is a parity-preserving Go adaptation of upstream's mutable array, without pointers to slices or a new public result type.
- Store one internal warning-aware callback. Adapt `WithRequestTransform` to preserve input warnings. Both options replace this slot, so the last one wins and nil disables the transform. No implicit chaining or precedence independent of option order.
- Invoke only in shared `buildRequest`, after all conversion warnings and stream fields are ready. Preserve the existing error wrapping and nil-body rejection. A callback returning an error prevents all HTTP calls.
- Bodies and warnings are call-local. Callbacks may run concurrently and must synchronize their own captured mutable state. The SDK does not store warning slices on the model or clone arbitrary callback-owned mutable state.
- Keep existing request metadata behavior: marshal the transformed body once and use the same bytes for HTTP and metadata. Core continues consuming the existing stream-start warning event; no core change is necessary.

## Risks / Trade-offs

Callbacks returning only newly created warnings intentionally replace existing warnings; godoc tells callers to append when retaining them. As with the existing callback, concurrent mutation of captured state is the caller's responsibility. The additive API name is longer than the old option but makes the capability explicit without a breaking signature change.

The core-consumer regression imports the root SDK package for the first time in this provider module's tests. Canonical `go mod tidy` adds its existing public transitive dependencies at unchanged versions; there is no new runtime dependency or baseline/published-version update.

## Validation Strategy

First add a regression test using the new option and record its failure against unchanged production code. Verify body replacement, current-warning input, returned-warning output and request metadata over unary/stream HTTP; legacy callbacks, nil disabling and last-option precedence; errors and nil bodies before HTTP; core GenerateText/StreamText consumers; sequential and concurrent isolation. Run focused/full provider tests, race, vet/lint, docs/spec and relevant registered parity checks. Synthetic responses prove local contracts only; no live-provider or new recording claim is made.
