# Design

## Context

- `providerwire/v4` owns everything about a language-model request today:
  - selection, on a worker goroutine raced against the context (`selection.go`);
  - unary invocation, on a worker with late-result discard (`handler.go`);
  - stream setup, on a worker with late-stream cleanup, plus the wait loop, idle timer and drain (`stream.go`);
  - classification into ProviderWire categories (`errors.go`);
  - attempt capture and the execution view (`execution_capture.go`);
  - host policy, interleaved with protocol mapping (`request.go`).
- `internal/execution` already holds the overview, metadata, native summaries and redaction (#480). This change adds the runtime beside them.
- #521 removed `ModelDuration`. Selection and the model call end at the request context's deadline, so `Limits` holds only the idle and drain durations.
- The App Platform host recovers handler-goroutine panics (plugin SDK `backend/serve.go`, gRPC recovery interceptor). At its route deadline it answers the caller itself, and anything the plugin writes afterwards is discarded.
- Configured selection is an in-memory catalog lookup. BYOK selection decodes the request and constructs a model. Neither does I/O.
- #428 removed the inbound header-refusing edge and its `TestInboundRefusedHeadersAreProtectedCallHeaders` test, but the `request.go` comment and two spec clauses still referred to them. This change fixes that drift.

## Goals / Non-Goals

**Goals:**
- One runtime that a second adapter can use with no edits to the runtime or ProviderWire.
- ProviderWire documents, statuses and SSE frames stay byte-identical for every request outside the removed policies.
- Less code: the worker goroutines and their handoff paths go away.

**Non-Goals:**
- One selector and one handler, and the route table (#482). Configured and BYOK keep separate runtimes.
- Moving host error categories (authentication, BYOK discovery) out of ProviderWire (#482).
- Lifecycle validation, which stays in ProviderWire's state machine (#483).

## Decisions

### D1. API: `Runtime` → `Call` → `Stream`, with errors classified by pure functions

```text
execution.New(Config{Selector, Limits, ConfiguredAPIKeys}) (*Runtime, error)
runtime.Call(ctx, modelID, options) (*Call, error)   // policy + selection
call.Generate() (*provider.GenerateResult, error)
call.Stream() (*Stream, error)                        // setup; drains on failure
call.View(err) View                                   // seals capture, overview + summaries
call.Close()                                          // seals capture
stream.Next() (provider.StreamPart, Wait)             // Part|Closed|Canceled|Deadline|IdleTimeout
stream.Close()                                        // cancel + one drain
execution.ClassifySelection(err) -> (Category, Reason); ClassifyProvider(err) / ClassifyStreamPart(err) -> Category
```

- The runtime returns ordinary Go errors: context errors, the host sentinels, new sentinels for policy refusals and internal failures, and provider errors.
- Adapters classify them with three pure functions. These are today's `safeErrorFromResolution`, `safeErrorFromProvider` and `safeErrorFromStreamProvider`, moved unchanged.
- This follows the repo rule of sentinels and `fmt.Errorf` only, with no custom error types.
- **Alternative: one result struct per step carrying the category.** Rejected: every step would need a parallel failure type, and adapters would lose the cause that the view and summaries need.
- **Alternative: one classifier for every error.** Rejected: a provider error that wraps a selection sentinel would change category.

### D2. Synchronous execution with context-outcome precedence

- Selection, `DoGenerate` and `DoStream` run on the caller's goroutine, inside one `recover`.
- After each returns, a request context that has ended wins:
  - the result is discarded;
  - a returned stream is canceled and drained once;
  - the context error is returned.
- A panic or a `nil, nil` result becomes an internal sentinel. A selection is validated as today.
- This removes the three worker goroutines and their channels. It also removes the "setup worker owns a late stream" handoff and the capture `discard` race guard, because a late outcome is now observed only after the call returns.
- **Alternative: keep the workers (issue #481).** Rejected. They only let the handler return before a model that ignores cancellation does. Through the platform, the caller is already answered at the route deadline, and provider HTTP calls honor the context.

### D3. Stream wait loop and drain

- `Stream.Next` waits for one of:
  - a part;
  - channel close;
  - the end of the request context, reported as `Deadline` or `Canceled` from its error;
  - the idle timer.
- Each call restarts the idle timer. Adapters call `Next` only after the previous part was represented, so this matches today's reset after each successfully represented part.
- Any non-part outcome cancels the model context before returning.
- `Close` is idempotent: it cancels, then starts one asynchronous drain bounded by the drain duration.
- The idle timer starts when setup returns the stream, as before, so the time to commit the response counts as idle time.

### D4. Host policy at the start of `Call`

- Mapping no longer reserves namespaces, so call-level `gateway` arrives in `options.ProviderOptions` as an ordinary `RawProviderOption`.
- `Call` first refuses protected call headers. It then removes call-level `gateway` and passes its raw bytes to the selector. The bytes are the same as today, because the schema already requires an object.
- The protected set and the `gateway` control namespace become `execution` constants. Config reads the namespace to refuse a `gateway` provider name.
- ProviderWire keeps the duplicate-spelling rejection and the dropping of `ai-language-model-*` and `ai-o11y-*` names.
- Precedence change: a protected header used to win over later mapping failures and now loses to them. Both are fixed 400 documents. The old order also depended on map order when a protected header and a duplicate header were both present.
- **Alternative: the mapper also calls the shared check to keep the old precedence.** Rejected: it duplicates the call site for no protective gain.

### D5. Removed namespace reservations

- `grafana`, `grafana-ai-sdk`, and `gateway` below call level are no longer reserved. No Gateway code reads them, and native models read only their own namespace.
- The openai-compatible model reads options under its `providerName`, so config now refuses `gateway` there instead of `grafana`. Otherwise call-level options for that backend would be taken as host controls.
- The self-hosted "reserved provider option namespace" 400 for call-level `gateway` is unchanged, and so is `docs/guides/gateway-authentication.md`.

### D6. Selection, capture and view in `execution`

- `Selection` (with unexported configured candidates), `RequestSelector`, `CatalogSelector` and the host sentinels move to `execution`. The cmd `service` selectors import them. This is allowed because `ai-gateway/internal` is visible to the whole module.
- Each `Call` attaches its own `fallback.WithAttemptObserver` for every selection, BYOK included. Publication comes from the existing `Project` rule: a BYOK selection has no requested or canonical identity, so its overview is nil.
- `fallback` stores one request observer per context and offers no way to read it, so the runtime's observer replaces one a caller placed on the request context. Configured calls already behaved this way. Model-level observers are unaffected, and no production code sets a context observer.
- **Alternative: chain to the caller's observer.** Rejected: it needs a new public getter in the root `fallback` module for no production caller.
- The view, redactor and one-candidate synthesis move unchanged.
- **Alternative: a BYOK-specific capture flag.** Rejected as unneeded until #420 publishes.

### D7. Limits without settings

- `execution.Limits{StreamIdleDuration, StreamDrainDuration}` is validated in `New`, and `execution.DefaultLimits` holds 30s and 1s.
- `Settings.ProviderWire` and `limitValidationResolver` are removed. Gateway wiring and the model observability factory read `DefaultLimits`, and tests pass short limits.
- **Alternative: keep the limits in `Settings`.** Rejected: they are not operator settings, and validating them meant constructing a handler.

### D8. ProviderWire becomes a codec

- `v4.New(runtime *execution.Runtime)` is the whole constructor.
- ProviderWire keeps:
  - envelope and schema validation;
  - mapping;
  - the category-to-document table;
  - its own capability reasons;
  - the host error writer;
  - commitment and SSE encoding;
  - the stream state machine.
- `safeError` maps `execution.Category` and `execution.Reason` into its existing table, with the host-only categories still local.

### D9. Test-only adapter

- `internal/execution` gets an external test package, `execution_test`, with a minimal line-protocol adapter over `Runtime`, so it can import `providerwire/v4` without a cycle.
- One scenario table drives both adapters with the same fake models:
  - selector and model panics, invalid selections and `nil, nil`;
  - late unary results and late streams;
  - idle, deadline and cancellation;
  - in-band stream errors;
  - protected headers and configured `gateway` controls;
  - fallback attempts.
- Each row expects a category and reason, model-call counts, drains, and an attempt view. The test adapter must report them directly, and ProviderWire must write the document for that category.

### D10. Test migration

Behavior is the gate:
- ProviderWire tests keep their inputs and expected documents, and change only construction and imports.
- White-box tests of moved internals move to `execution` with the same assertions.
- Tests that asserted bounded handler latency for work that ignores cancellation become late-result tests.
- Tests asserting reserved-namespace refusals become pass-through tests.

## Risks / Trade-offs

- [A selector or model that ignores cancellation holds its request goroutine until it returns] → The platform still answers the caller at its deadline, providers honor the context, and the testserver sets deadlines. Documented in the godoc and the specs.
- [Without a platform deadline, direct HTTP and testserver callers wait for a misbehaving model] → Test-only, and covered by the lifecycle-defect requirement.
- [Nested `gateway`, `grafana` and `grafana-ai-sdk` options now reach native middleware and telemetry] → They are caller content like any other namespace. Call-level BYOK keys are still removed before selection.
- [Precedence change between protected headers and mapping failures] → Both are fixed 400 documents with no echo, and the cases are covered by tests.
- [A missed behavior while moving code] → The ProviderWire suites, the test adapter's shared table and `go test -race` gate each commit.

## Migration Plan

- No operator migration. A configured `providerName: gateway` fails at startup with the existing `providers.<name>.providerName` error.
- No consumer outside the `ai-gateway` module uses the moved Go API.
- Rollback is reverting the PR.
