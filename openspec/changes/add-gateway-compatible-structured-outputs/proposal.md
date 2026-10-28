## Why

The Gateway constructs every `openai-compatible` backend without `WithStructuredOutputs`, so a request with a response schema reaches the backend as `response_format: {type: "json_object"}` and carries an `unsupported` warning, even when the backend implements JSON-schema outputs. Upstream `@ai-sdk/openai-compatible` 3.0.71 lets the caller opt in per provider at construction (`supportsStructuredOutputs`, default `false`). The Gateway has no setting for this, so operators cannot opt in and the #201 matrix rows `openai-compatible/recorded/structured-json-output` fail (issue #561).

## What Changes

- Add an optional boolean provider key, `providers.<name>.supportsStructuredOutputs`, to the Gateway command model catalog. It defaults to `false`. It is valid only for `type: openai-compatible`; setting it to `true` on `anthropic` or `openai` fails at startup with the same field-named error used for `providerName`.
- Carry the setting through provider resolution and pass it to `openaicompatible.WithStructuredOutputs` when the catalog constructs compatible models.
- With the setting on, schema requests reach the backend as `json_schema` with upstream's name and strict defaults, and no warning is added. With it off, behavior and the warning are unchanged.
- Document the key in the operator catalog reference, and correct the Gateway structured-output page, which currently says compatible providers always use JSON mode.
- Prove the setting on main through service tests (per-candidate settings, the per-call `strictJsonSchema` override, mixed fallback) and the command integration test through the Go and TypeScript clients.
- Harness toggle. When the #201 harness catalog builder is on main, set the key for `openai-compatible` rows in this change. Otherwise the toggle goes into #201 after this change merges. The toggle applies to every compatible matrix row, so the whole compatible matrix must stay green. Only that matrix-row verification is then pending #201.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-provider-configuration`: compatible provider configuration accepts `supportsStructuredOutputs` and rejects it on other types. Compatible model construction passes the capability through and keeps the default `false`.
- `gateway-structured-output`: existing native output semantics gain explicit compatible-backend scenarios for the setting on (`json_schema`, no warning) and off (`json_object` plus the unchanged warning).

## Impact

- `ai-gateway/cmd/grafana-ai-gateway/internal/config/file.go`: `Provider`, `ResolvedProvider`, `Validate` and `ResolveProviderSecrets`, plus `file_test.go`.
- `ai-gateway/cmd/grafana-ai-gateway/internal/service/catalog.go`: compatible construction, plus `structured_output_test.go` and an additive helper next to `nativeOptionsRequest` in `native_options_test.go`.
- `ai-gateway/test/providerwire-v4/gateway-command.test.ts`: a compatible structured-output case through both clients.
- Configured discovery output is unchanged.
- Docs: `ai-gateway/README.md` (Configure models) and `docs/providers/grafana-gateway.md` (Structured output).
- No change to `providers/openai-compatible`, to the wire format, or to dependencies. Existing catalogs keep their current behavior. No `PARITY.md` or `knownGaps` edit is expected (see design). The pre-existing off-path warning-text divergence is tracked through an `upstream-sync` issue.
- Harness: `test/conformance/tools/gateway-runtime.mts` `catalog()` exists only on the #201 branch, so this file is changed only if #201 has merged first.
