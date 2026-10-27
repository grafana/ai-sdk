# Proposal

## Why

After #518, four Gateway limits duplicate or contradict the platform, and one concept has two names (#521, part of #394):

- **Request deadline.** The plugin host ends every route request after 60s, streams included (#460). The 120s `max_model_duration` default can never be reached, and the authentication guide wrongly says streaming avoids the 60s. App Platform is adding a configurable route deadline, so the Gateway does not need a second one.
- **Request size.** The plugin receives each `CallRoute` request, body included, as one gRPC message, which the plugin SDK caps at 16 MiB. The Gateway sets no `GRPCSettings`. The 1 MiB `max_request_bytes` is a stricter duplicate of that cap and is too small for inline file inputs.
- **Provider response size.** Memory for a stream grows with its largest event, not with its total bytes, and time already bounds the total. Today a long, valid stream fails once it passes 16 MiB in total.
- **Attempts.** Native retries are off and `fallback` tries each candidate once, so a chain's length is its number of provider attempts. Two constants bound that one concept: `maxCandidates` (16, configured chains) and `maxCredentials` (8, BYOK). The count does not bound time, because the request deadline covers the whole chain. It bounds fast-failure amplification: N quick failures become N outbound requests.
- **Memory.** Total memory is per-request memory times concurrency, and no layer documents an owner for it.

## What Changes

- **BREAKING (operator): platform request deadline.**
  - Remove the `max_model_duration` setting.
  - Selection and the model call end at the request context's deadline, which the platform's route deadline sets through the plugin route RPC.
  - The 30s stream idle and 1s drain durations stay.
  - A request context that ends with a deadline is classified as a timeout, and one that is canceled as a cancellation.
- **BREAKING (operator): platform request size.**
  - Remove the `max_request_bytes` setting, `Settings.MaxRequestBytes` and the route `http.MaxBytesHandler` wrapper.
  - The plugin host's route message size bounds the request. An oversized request gets the host's error format, not the ProviderWire invalid-request document.
  - The ProviderWire testserver keeps its own fixed bound, and ProviderWire still maps a host's `*http.MaxBytesError` to invalid request.
  - A value set for a removed setting is ignored.
- **One attempt bound.** One constant, 8 provider attempts per request, replaces `maxCandidates` and `maxCredentials`:
  - configured models: primary plus fallbacks ≤ 8, checked at config load with today's configuration error;
  - BYOK: selected-provider accounts ≤ 8, checked at decode with today's fixed BYOK 400 and zero native calls. Unused provider arrays are still not counted.
- **Non-stream response bound.** The shared model transport:
  - bounds a non-stream response at 16 MiB in total, counted after decompression, as today;
  - no longer bounds a `text/event-stream` response, which the provider decodes incrementally and time bounds; #546 bounds one event in the openai-compatible decoder;
  - keeps the 10s header timeout and redirect rejection.
- **Memory owner.** The limit ledger states that the platform owns concurrency and process memory, and gives generic guidance: set `GOMEMLIMIT` to about 85–90% of the container memory limit. The Gateway adds no limiter on concurrent requests.
- **BREAKING (Go API):** `providerwire/v4.Limits` loses `ModelDuration`.
- **Test host.** The platform test host can apply a route deadline to the route RPC, so tests can prove that a host deadline reaches the plugin's request context.
- **Docs.** Update the ledger in `ai-gateway/README.md`, the settings table, `docs/providers/grafana-gateway.md`, the architecture walkthrough, the glossary and the godoc. Fix the 60s contradiction in `docs/guides/gateway-authentication.md`.

### Revised #394 and #518 decisions

- "`max_request_bytes` and `max_model_duration` keep their names and meaning" no longer holds: both settings are removed.
- #518 design D1 ("Rely on the 16 MiB host message size: rejected") is reversed. `design.md` D2 answers both of its reasons.
- "The Gateway adds no concurrency bound of its own" still holds. The ledger now names the platform as the memory owner.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-resource-policy`: replaces the route request-body bound with the platform request size, the model duration with the platform request deadline, and drops the byte bound on streamed responses. Adds the shared attempt bound and the memory owner to the ledger.
- `providerwire-v4-unary-runtime`: construction takes only idle and drain durations. The execution deadline is the request context's deadline.
- `providerwire-v4-streaming-runtime`: streaming limits drop the total model duration. Setup and consumption use the request context's deadline, and its expiry is a timeout.
- `gateway-request-byok`: the account bound is the shared attempt bound, and the request size is platform-owned.
- `gateway-configured-discovery`: a configured chain allows 8 candidates instead of 16.
- `gateway-provider-configuration`: the shared transport bounds each non-stream response and leaves streams to time.
- `gateway-file-inputs`: the platform route message size bounds file request bytes, not a Gateway setting.
- `gateway-streaming-function-tools`: history-derived state is bounded by the platform request size.
- `gateway-plugin-host`: deployment guidance uses the platform deadline, and the test host may prove deadline propagation through the route RPC.

## Impact

- **Code:** `cmd/grafana-ai-gateway/internal/{config,gateway,byok,outbound}`, `providerwire/v4` (`handler.go`, `stream.go`, `doc.go`), `test/platformhost`, `test/providerwire-v4/testserver`, and the affected Go and TS tests.
- **Operators:**
  - `max_request_bytes` and `max_model_duration` stop having any effect, and an invalid value no longer fails startup;
  - requests up to the host message size are accepted;
  - a configured model with more than 7 fallbacks now fails at startup;
  - streams longer than 16 MiB in total no longer fail.
- **Go API:** `v4.Limits.ModelDuration` is removed, so the PR title carries `!`. There is no consumer outside the `ai-gateway` module.
- **Wire:** no change to the ProviderWire documents. A request above the host message size now gets the host's error instead of the ProviderWire 400.
- **Parity:** Gateway resource policy is a Go extension. `@ai-sdk/gateway@4.0.103` defines only the client. `knownGaps`, recorded provider fixtures and conformance expectations are unchanged.
- **Delivery:** branch `nrbrd/gateway-platform-limits`, based on `nrbrd/gateway-resource-policy` (#518). #481 bases on this branch.
- **Out of scope:**
  - extracting the execution runtime (#481) and the host contract (#482);
  - the platform's configurable route deadline itself (#460);
  - platform admission and concurrency (#104);
  - publishing BYOK attempt overviews (#420).
