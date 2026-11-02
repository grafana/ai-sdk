## Why

The Gateway constructs every `openai-compatible` backend without `WithStructuredOutputs`, so a request with a response schema reaches the backend as `response_format: {type: "json_object"}` and carries an `unsupported` warning, even when the backend implements JSON-schema outputs. Upstream `@ai-sdk/openai-compatible` 3.0.71 lets the caller opt in per provider at construction (`supportsStructuredOutputs`, default `false`). The Gateway has no setting for this, so operators cannot opt in (issue #561).

## What Changes

- Add an optional boolean provider key, `providers.<name>.supportsStructuredOutputs`, to the Gateway command model catalog. It defaults to `false`. It is valid only for `type: openai-compatible`; setting it to `true` on `anthropic` or `openai` fails at startup with the same field-named error used for `providerName`.
- Carry the setting through provider resolution and pass it to `openaicompatible.WithStructuredOutputs` when the catalog constructs compatible models.
- With the setting on, schema requests reach the backend as `json_schema` with upstream's name and strict defaults, and no warning is added. With it off, requests still fall back to `json_object` with an `unsupported` `responseFormat` warning.
- Align that off-path warning's `details` in `providers/openai-compatible` with upstream 3.0.71 (`JSON response format schema is only supported with structuredOutputs`). This fixes a pre-existing divergence on the path this change touches, so direct and Gateway callers see upstream's text.
- Document the key in the operator catalog reference, and correct the Gateway structured-output page, which currently says compatible providers always use JSON mode.
- Prove the setting through provider tests, service tests (per-candidate settings, the per-call `strictJsonSchema` override, mixed fallback) and the command integration test through the Go and TypeScript clients.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-provider-configuration`: compatible provider configuration accepts `supportsStructuredOutputs` and rejects it on other types. Compatible model construction passes the capability through and keeps the default `false`.
- `gateway-structured-output`: existing native output semantics gain explicit compatible-backend scenarios for the setting on (`json_schema`, no warning) and off (`json_object` plus the upstream-aligned warning).

## Impact

- `ai-gateway/cmd/grafana-ai-gateway/internal/config/file.go`: `Provider`, `ResolvedProvider`, `Validate` and `ResolveProviderSecrets`, plus `file_test.go`.
- `ai-gateway/cmd/grafana-ai-gateway/internal/service/catalog.go`: compatible construction, plus `structured_output_test.go` and an additive helper next to `nativeOptionsRequest` in `native_options_test.go`.
- `ai-gateway/test/providerwire-v4/gateway-command.test.ts`: a compatible structured-output case through both clients.
- `providers/openai-compatible/convert_request.go`: the off-path warning `details` text, plus a provider test ported from upstream. The Gateway builds this provider from same-revision source (`ai-gateway/go.mod` replace), so both modules change together.
- Configured discovery output is unchanged.
- Docs: `ai-gateway/README.md` (Configure models) and `docs/providers/grafana-gateway.md` (Structured output).
- No change to the wire format or dependencies. Existing catalogs keep their request behavior; only the off-path warning `details` text changes. No `PARITY.md` or `knownGaps` edit is expected (see design).
- This change closes #561. The Gateway conformance matrix harness is separate work and outside this change.
