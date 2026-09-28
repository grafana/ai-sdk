## Why

At the registered `@ai-sdk/anthropic@4.0.59` baseline, callers can opt in to dangerous-tool-use classification and read the returned per-tool-call verdicts. Go's Anthropic adapter currently supports neither the request nor the response projection, so Go callers cannot use this optional capability. This does not imply that ordinary Anthropic calls are unsafe.

## What Changes

- Expose optional `safeguards` on model-level Anthropic provider options, with a `dangerous_tool_use` entry and optional `classifierContext` passed to Anthropic as `classifier_context`.
- Send safeguards and the exact `dangerous-tool-use-2026-09-03` beta only for nonempty requests, for both unary and streaming calls; preserve omission for absent/empty options.
- Expose validated, wire-shaped `safeguard_results` as optional `providerMetadata.anthropic.safeguardResults` in unary and streaming finishes; retain the last non-null streaming verdict, without exposing unrelated raw response fields.
- Add focused fake-HTTP/request, response, stream and error tests. Keep provider-live support and permissions an explicit prerequisite for enabling this opt-in in production; no live validation is claimed by synthetic tests.

## Capabilities

### New Capabilities

- `anthropic-safeguards`: Anthropic-specific opt-in safeguard requests, automatic beta, and provider verdict metadata for unary and streaming results.

### Modified Capabilities

None.

## Impact

- `providers/anthropic/options.go`, `convert_request.go`, `model.go`, `provider_metadata.go`, `convert_response.go` and `convert_stream.go` and their focused tests. The `provider.LanguageModel` interface, generic provider result shapes, and SSE chunk discriminators remain unchanged.
- The current separate Anthropic module depends on `github.com/anthropics/anthropic-sdk-go@v1.75.0`, which has no typed safeguards request/response fields; request serialization and raw-response handling must be verified against that SDK. A dependency bump, if ultimately needed, must independently resolve as a published version with `GOWORK=off`.
- This is provider-implementation parity against registered source commit `4e8c387622ee1bb0d55841664416d38754d5c9a3`, not a pinned-version upgrade or evidence of live provider acceptance.
