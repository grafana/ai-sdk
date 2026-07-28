## 1. Client option types (providers/grafana)

- [x] 1.1 Define the `CaptureMode` named string enum type with typed constants mirroring the server-side sigil capture-mode set (full, metadata-only, full-with-metadata-spans)
- [x] 1.2 Define `SigilControl` struct carrying `Disabled *bool` and the `CaptureMode` field
- [x] 1.3 Define `GrafanaOptions` struct with a `*SigilControl` field (and nil-able placeholders for future Tracing/Metrics/Usage controls per design); ensure every control struct carries `Disabled *bool`
- [x] 1.4 Implement `ProviderKey()` returning `"grafana"` and add a compile-time `var _ provider.ProviderOption = GrafanaOptions{}` check
- [x] 1.5 Implement client-side `Validate()` that rejects an out-of-set `CaptureMode` and accepts defined constants

## 2. Client option tests (providers/grafana)

- [x] 2.1 Test `GrafanaOptions` satisfies `provider.ProviderOption` with key `"grafana"`
- [x] 2.2 Test lossless round-trip: marshal via `provider.ProviderOptions`, unmarshal, recover via `provider.ResolveOption[GrafanaOptions]`
- [x] 2.3 Test marshal emits the `"grafana"` key with concrete `GrafanaOptions` JSON
- [x] 2.4 Test nil control fields are omitted / mean "no preference"
- [x] 2.5 Test client-side validation rejects invalid `CaptureMode` and accepts valid ones
- [x] 2.6 Test `Disabled *bool` round-trips and that nil vs true vs false are distinguishable after the wire boundary

## 3. Client documentation (providers/grafana)

- [x] 3.1 Document attaching `GrafanaOptions` via `BuildProviderOptions` + `WithProviderOptions` in the README, including the sensitive-conversation metadata-only example
- [x] 3.2 Document that there is intentionally no context helper for these options

## 4. Verification

- [x] 4.1 Run `make check` in this repo (fmt, vet, tests) for `providers/grafana`
- [x] 4.2 Confirm the wire contract is additive: existing clients sending no `GrafanaOptions` produce unchanged behavior end-to-end

## Follow-up (separate `grafana-assistant-app` PR, out of scope here)

The backend resolution and enforcement is delivered after this change merges and
is synced into the backend's ai-sdk dependency. Captured here for traceability;
not tracked by this change:

- Resolver helper reading `opts.ProviderOptions["grafana"]` via `provider.ResolveOption[GrafanaOptions]`, with strict `DisallowUnknownFields` decoding and 4xx non-retryable `APICallError` on unknown field or invalid enum.
- Sigil middleware: client-preference precedence over the tenant default, `CaptureMode` mapping to `sigil.ContentCaptureMode`, and `Disabled` short-circuit before `StartGeneration` (Disabled wins over CaptureMode).
- A shared per-middleware `Disabled` short-circuit convention for tracing/metrics/usage as those middlewares land.
- Backend tests for resolution, strict rejection, precedence, isolation, and the disable short-circuit.
