# Design

## Context

See proposal.md for motivation. This change builds on #518 (`nrbrd/gateway-resource-policy`), which moved every resource limit to a shared boundary.

Current state, by limit:

| Limit | Where | Value |
| --- | --- | --- |
| `max_model_duration` | `v4.Limits.ModelDuration`, applied with `context.WithTimeout(r.Context(), …)` in `handler.ServeHTTP` | 120s default, at least 30s |
| `max_request_bytes` | `Settings.MaxRequestBytes`, `http.MaxBytesHandler` around the mux in `gateway.newHandler` | 1 MiB default |
| Provider response bytes | `outbound.boundedTransport`, total per response | 16 MiB |
| Configured candidates | `config.maxCandidates`, checked as `len(Fallback) >= 16` | 16 |
| BYOK accounts | `byok.maxCredentials` in `DecodeRequest` and `New` | 8 |

Code facts that shape the approach:
- **Route requests arrive whole.** The app-sdk route adapter receives `CallRouteRequest` (body included) as one gRPC message. It builds the `*http.Request` from `stream.Context()` and `bytes.NewReader(req.GetBody())`.
  - `plugin.Run` serves with the plugin SDK's default `MaxRecvMsgSize` of 16 MiB (`defaultServerMaxReceiveMessageSize`), because the Gateway sets no `GRPCSettings`.
  - A larger message fails with gRPC `ResourceExhausted` before any Gateway code runs.
- **Deadlines and cancellation cross the RPC.** `httpadapter.HandlerFunc` passes `r.Context()` to `CallRoute`, and gRPC carries a context deadline as `grpc-timeout`. The plugin's `stream.Context()`, and therefore `r.Context()`, gets that deadline. Caller cancellation also reaches it.
- **Expiry arrives as a cancellation.** When the route deadline expires, the caller's gRPC client cancels the stream before the plugin's own deadline timer fires. In the test host, the plugin's request context ended with `context.Canceled` in 20 of 20 runs, once before its copy of the deadline had passed. The plugin cannot tell a platform deadline from a caller disconnect.
- **The in-process test host** (`platformhost.ServeInProcess`) uses `grpc.NewServer()` with gRPC's 4 MiB receive default. Only the built plugin, launched through the process host, has the production 16 MiB limit.
- **Streaming classifies termination by which context ended.** `serveStream` receives `requestContext` and `executionContext`. When the model context ends, `waitStreamPart` and `streamContextErrorCategory` report cancellation if `requestContext.Err() != nil`, and a timeout otherwise. Today, then, an earlier request deadline is reported as cancellation. The unary path already maps `context.DeadlineExceeded` to timeout in `safeErrorFromProvider`.
- **Stream decoders buffer one event at a time.** The Anthropic and OpenAI SDK SSE decoders cap a line at 32 MiB (`bufio.MaxScanTokenSize<<9`) and join an event's `data:` lines. The openai-compatible reader uses an unbounded `bufio.Reader` and `strings.Builder`.
- **Import graph.** `config` imports `auth` and `providerwire/v4`. `byok` imports `nativemodel` and `nativeoptions`. Neither imports the other.

## Goals / Non-Goals

**Goals:**
- The request context's deadline is the only total deadline, and its expiry is reported as a timeout on both paths.
- One attempt bound, with today's error behavior at both check sites.
- The transport bounds what the SDKs read whole: one non-stream response. Streams are bounded by time.
- Tests prove that a host deadline crosses the route RPC, and that request size is enforced by the real plugin.

**Non-Goals:**
- Changing the ProviderWire documents, the stream idle or drain durations, or the 10s header timeout.
- Raising or setting the plugin's gRPC receive size.
- Moving the attempt bound into `fallback` or ProviderWire (#481).
- Any in-process concurrency or memory limiter.

## Decisions

### D1. The request context's deadline is the execution deadline
- `v4.Limits` keeps `StreamIdleDuration` and `StreamDrainDuration`. `ServeHTTP` derives no timeout: selection and invocation use `r.Context()`, which still carries the attempt observer as today.
- `validateLimits` checks only the two durations.
- **Classification follows the request context's error.** `context.DeadlineExceeded` is a timeout and `context.Canceled` is a cancellation, on both paths.
  - Unary already does this through `safeErrorFromProvider`.
  - Streaming checks `errors.Is(requestContext.Err(), context.DeadlineExceeded)` in `waitStreamPart` and `streamContextErrorCategory`. Without the check, an expired deadline that a host delivers as `DeadlineExceeded` would be reported as a cancellation in streaming only.
  - Through the plugin host, a platform deadline arrives as a cancellation (see Context), so the Gateway reports it as one. The ledger states this.
- **Settings.**
  - `config.ParseSettings` stops reading `max_model_duration` and `max_request_bytes`. `Settings.MaxRequestBytes`, `defaultRequestBytes` and `defaultModelDuration` go.
  - `Validate` drops the "at least 30s" rule and still builds a throwaway handler to check the fixed durations.
  - Unknown `GF_PLUGIN_*` variables are already ignored, so a removed setting is ignored without extra code.
- **Platform deadline disabled.** With no deadline on the request context, the call ends only at completion, caller cancellation, an attempt's 10s header timeout or the 30s stream idle duration. A unary attempt whose provider stalls after sending headers is bounded only by caller cancellation. The ledger states this.
- **Anthropic unary calls need a deadline.** `anthropic-sdk-go` refuses a non-streaming call whose `max_tokens` it estimates above 10 minutes (about 21,333 tokens, or a lower per-model limit) unless it has a request timeout. `providers/anthropic` supplies that timeout from the context deadline (`anthropic-unary-context-deadline`), and the removed `ModelDuration` always provided one. Without a platform deadline, such a call fails with the internal error before any native request. Streaming is unaffected, and `@ai-sdk/anthropic@4.0.71` has no such guard.
- **Test host deadline.** The command tests run the built plugin behind the test host with a 60s route deadline (`--route-deadline`), as Grafana serves it today.

Alternatives:
- *Keep `ModelDuration` with a default above the platform deadline.* Rejected: the setting can never be reached, and it would contradict the platform once #460 makes the deadline configurable.
- *Keep a fixed internal ceiling for when the platform deadline is disabled.* Rejected: it reintroduces the owner conflict, and it would cut off the long streams an operator disables the deadline to allow. Disabling the deadline is an operator choice that the platform owns.
- *Always pass the Anthropic SDK a request timeout, as `@ai-sdk/anthropic` effectively behaves.* Deferred: it reverses `anthropic-unary-context-deadline` in the Anthropic provider module and belongs in its own change.
- *Keep today's streaming classification, where an ended request context means cancellation.* Rejected: unary and streaming would disagree for a host that delivers `DeadlineExceeded`.
- *Report a timeout whenever a request with a deadline ends.* Rejected: it mislabels caller disconnects as timeouts.
- *Report a timeout when the context ends close to its deadline.* Rejected: it needs an invented tolerance, and the observed cancellation can precede the plugin's copy of the deadline.

### D2. Request size is the host's route message size (reverses #518 D1)
Remove the `http.MaxBytesHandler` wrapper and the `maxRequestBytes` parameter of `gateway.newHandler`. ProviderWire's `readBody` still maps `*http.MaxBytesError` to the fixed invalid request, so a host that applies its own bound keeps today's document. The ProviderWire testserver keeps `http.MaxBytesHandler` with its local 1 MiB constant.

#518 D1 rejected relying on the host message size for two reasons. They are answered as follows:
1. **"Up to 16 MiB of JSON would reach schema validation."** True, and accepted.
   - The host has already received, buffered and decoded those bytes into a `[]byte` before the Gateway runs. The 1 MiB bound only saved validation CPU and the JSON decode tree, not receive memory.
   - Validation is linear in input size, and 16 MiB is the same order as the provider responses the Gateway already decodes.
   - Per-request memory times concurrency is a platform concern (#104). The ledger names it, together with the `GOMEMLIMIT` guidance.
   - Meanwhile the 1 MiB bound rejected valid requests: with base64 and JSON escaping, an image of about 750 KiB already exceeds it.
2. **"A handler served elsewhere would have no bound."** The Gateway has no other serving surface (gateway-plugin-host, "No standalone server surface").
   - The only other host of the ProviderWire handler is the testserver, and it keeps its bound.
   - The package godoc already makes the body bound a host responsibility. A future host (#482) declares its bound as part of the host contract.

An oversized request fails with gRPC `ResourceExhausted`, which the host renders in its own format. The test host's `httpadapter` returns a 429 Kubernetes `Status`, while Grafana's format is its own. The specs therefore only require a non-ProviderWire rejection with zero model calls.

*Alternative: set `GRPCSettings.MaxReceiveMsgSize` explicitly.* Rejected: the size belongs to the platform's route contract, and a Gateway value would become a third owner.

### D3. One attempt bound constant in `config`
- Add `config.MaxProviderAttempts = 8`, next to `ProviderResponseBytes` and `ProviderResponseHeaderTimeout`, which `config` already owns as fixed Gateway limits.
- The catalog loader checks `len(model.Fallback)+1 > MaxProviderAttempts` with today's cardinality error.
- `byok` imports `config`. `DecodeRequest` checks the selected provider's array against it, still returning `ErrInvalidRequest` (mapped to the fixed BYOK 400). `byok.New` keeps the same check as a constructor invariant.
- `maxCandidates` and `maxCredentials` go.

Alternatives:
- *`nativemodel.MaxAttempts`.* Rejected: `config` would import the provider SDK packages that `nativemodel` pulls in.
- *A new `limits` package.* Rejected (YAGNI): it would hold a single constant.
- *Pass the bound as a parameter.* Rejected: it adds plumbing for a value that never varies.

### D4. Streams are not byte-bounded in the shared transport
`boundedTransport.RoundTrip` checks the response's media type (`mime.ParseMediaType`, case-insensitive) after the base transport's transparent decompression. A `text/event-stream` body passes through unwrapped. Any other media type, or a missing or unparsable one, keeps the existing total `boundedBody`.

- **Why not bound streams:** the provider decodes a stream incrementally, so its memory is the largest event the decoder buffers, not the total. The Anthropic and OpenAI SDK decoders cap a line at 32 MiB. Time bounds the total through the request deadline and the 30s idle duration.
- **The remaining gap:** the openai-compatible reader buffers a line and an event without a bound, so a misbehaving configured backend could grow one request's memory until time ends it. BYOK reaches only the official endpoints. #546 bounds the event in that reader, where it is buffered.
- **Why the non-stream bound stays:** the SDKs read a non-stream body whole, and no other layer bounds it.

Alternatives:
- *Raise the total bound.* Rejected: it still ends long, valid streams, and it does not bound memory any better.
- *Bound each event in the transport.* Implemented first, then rejected in review: it is a second SSE parser with its own line-ending state, duplicating the decoders, to guard a configured backend's bug. The bound belongs in the decoder that buffers the event (#546).
- *Bound lines instead of events.* Rejected: decoders accumulate multi-line `data:` fields per event, so a line bound does not bound memory.

### D5. The test host can apply a route deadline
`platformhost.Config` gains `RouteDeadline time.Duration`. When it is positive, `Host.Handler` wraps the forwarded context in `context.WithTimeout` before `CallRoute`, as a platform route deadline would. Zero means no deadline. The test host stays test-only, and it is not evidence of Grafana's deadline value.

Evidence:
- **`platformhost` host test, with a stub route handler:**
  - the handler's `r.Context()` carries a deadline no later than `RouteDeadline`;
  - without `RouteDeadline`, it carries none.
- **Gateway route tests (in-process host), with a fake native provider that blocks until its request is canceled:**
  - in unary and streaming mode, the native request is canceled at the route deadline;
  - a two-candidate chain starts no second attempt;
  - without a deadline, cancelling the caller still cancels the native request.
- **ProviderWire handler unit tests:** the timeout and cancellation categories. Through a host, the caller sees the host's deadline error, not the Gateway document.

### D6. Request-size and long-stream evidence uses the built plugin
The in-process host's 4 MiB gRPC default differs from the production 16 MiB, so size evidence runs in `gateway-command.test.ts` through the process host with the built plugin. Both registered clients are covered:
- a request of about 2 MiB with an inline file succeeds;
- a request above 16 MiB fails before selection, and the fake native provider counts zero calls;
- a fake Anthropic stream totalling about 20 MiB in events of about 1 MiB is received completely.

The stream pass-through and non-stream overflow cases are `outbound` unit tests. The existing large-output test keeps its values.

### D7. Ledger and docs
The `ai-gateway/README.md` Limits table becomes two groups.

**Gateway:**

| Limit | Owner |
| --- | --- |
| 8 provider attempts per request: a model's primary and fallbacks, or a request's keys for the selected provider | Catalog loader and request key selection |
| 16 MiB per provider response that is not a stream, counted after decompression; 10s to receive headers; no redirects | Model transport |
| 30s without stream activity; 1s of cleanup after a stream ends | Language-model handler |
| Catalog file, IDs, aliases, 7 fallbacks, string lengths | Model catalog loader (unchanged, fallback count updated) |

**Platform:**

| Limit | Owner and source |
| --- | --- |
| Request deadline, streams included | Grafana's route deadline: 60s today, configurable with #460 |
| Request size | Plugin route message size: 16 MiB, the plugin SDK default |
| Admission and concurrency | Grafana (#104) |
| Process memory | The container memory limit. Set `GOMEMLIMIT` to about 85–90% of it. |

Then:
- **Removed limits:** `max_request_bytes`, `max_model_duration`, the total size of a streamed response, and the #518 list, each with its replacement.
- **Disabled platform deadline:** what still bounds a call (D1).
- **Deployment:** the termination grace period must exceed the platform request deadline plus the 1s drain.

The Settings table drops both rows. Other docs:
- **`docs/guides/gateway-authentication.md`:** the 60s row says the deadline includes streams, and drops "Stream long responses".
- **`docs/providers/grafana-gateway.md`:** the raw-events paragraph says time, not size, bounds a provider stream. "Bound work and handle errors" names the platform as the owner of request size and duration.
- **`docs/concepts/gateway-architecture.md`:** "model deadline" becomes "request deadline" in the sequence diagram, step 3 and the limits paragraph.
- **`docs/concepts/gateway-glossary.md`:** the deadline row changes the same way.
- **`providerwire/v4` godoc** (`doc.go`, `Limits`, `Config`): the handler bounds only idle and drain time, and uses the request context's deadline.

### D8. Removed-identifier check
`rg -w 'ModelDuration|max_model_duration|MaxRequestBytes|max_request_bytes|maxCandidates|maxCredentials'` runs over `ai-gateway/`, `docs/` and `openspec/specs/` after the change is archived. `CHANGELOG.md` and archived changes are excluded.

The ledger's removed-limit row is the one intended historical reference to the two setting names. The command test that rejects a malformed setting switches from `--max-model-duration` to another typed setting, `agento11y_enabled`.

### D9. Parity
This is a Go extension with no upstream counterpart: `@ai-sdk/gateway@4.0.103` defines only the client. No `knownGaps`, recorded fixtures or expectations change, and `mise run validate-parity-baseline` confirms the baseline is untouched.

## Risks / Trade-offs

- **Platform deadline disabled** → A unary attempt that stalls after its headers holds a goroutine and a connection until the caller disconnects.
  - Mitigation: the ledger documents it. The platform owns the deadline (#460), and streams keep the idle bound.
- **Without a platform deadline, unary Anthropic calls with a large token budget fail** → A call that leaves `maxOutputTokens` unset on a model with a large default budget gets the internal error.
  - Mitigation: Grafana sets a route deadline today. The ledger tells operators to set `maxOutputTokens` or stream, and the provider-level fix is deferred (D1).
- **A platform deadline shows as a cancellation in Gateway logs, metrics and attempt records** → Operators cannot tell it from a caller disconnect in Gateway telemetry.
  - Mitigation: the caller gets the host's deadline error, not the Gateway's. The ledger states that Grafana's own request logs show the deadline.
- **Requests up to 16 MiB reach schema validation** → More CPU and decode memory per request than with a 1 MiB bound.
  - Mitigation: the ledger's memory section, the `GOMEMLIMIT` guidance and platform admission (#104).
- **Catalogs with 8 to 15 fallbacks stop loading** → A breaking change for those operators, with today's generic cardinality error.
  - Mitigation: the `!` PR title, the ledger row and the migration step below.
- **A streaming provider that sends the wrong media type** → The total bound applies, and a long stream fails as it does today.
  - Mitigation: Anthropic, OpenAI and common compatible backends send `text/event-stream`. The outcome is fail-safe and no worse than today.
- **The in-process host's receive limit differs from production** → In-process tests cannot show the 16 MiB rejection.
  - Mitigation: D6 uses the built plugin for size evidence.

## Migration Plan

1. Operators remove `max_request_bytes` and `max_model_duration` from the `[plugin.grafana-aigateway-app]` section. Values left in place are ignored.
2. Operators shorten any configured model to at most 7 fallbacks before upgrading.
3. Go code that builds `v4.Limits` drops `ModelDuration` and applies a deadline on the request context instead.
4. Rollback is a revert of this PR. There is no persisted state.

