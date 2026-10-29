## Context

See proposal.md (Why). State before this change, checked on this worktree:

- `config/file.go:33-39` `Provider` has camelCase YAML keys, and `LoadFile` decodes them strictly with `KnownFields(true)` (L108). `ResolvedProvider` (L57-62) is built in exactly one place, `ResolveProviderSecrets` (L253).
- `Validate` (L135-159) switches on the provider type. Its only type-restricted field today is `providerName`, rejected on `anthropic` and `openai` at L136-139 with `config: providers.%s.providerName is only supported for openai-compatible providers`.
- `service/catalog.go:62-73` builds compatible candidates with `WithAPIKey`, `WithBaseURL`, `WithHTTPClient`, `WithProviderName` and `WithIncludeUsage(true)`. It never calls `WithStructuredOutputs`.
- `providers/openai-compatible` already implements the switch. `WithStructuredOutputs` is at `options.go:69-71`, the model field at `model.go:34`, and the mapping in `convertResponseFormat` (`convert_request.go:818-858`):
  - no schema → `json_object`
  - schema with the switch off → `json_object` plus warning `JSON schema response format requires WithStructuredOutputs(true); using json_object.` (L833-839)
  - schema with the switch on → `json_schema` with `strict` defaulting to `true` unless `StrictJSONSchema` overrides it (L841-844), `name` defaulting to `"response"` only when absent (L845-848), and the description and schema.
- Per-call compatible options are read from `openaiCompatible` and the camelCase provider-name namespace (`convert_request.go:192-200`; `myVllm` for `my-vllm.chat`). The raw `openai-compatible` and `my-vllm` keys still work but add a deprecation warning (`convert_request.go:230-240`).
- `service/structured_output_test.go:12-55` runs backend × stream × fallback × name. Its `openai-compatible` case (L45-47) asserts `json_object` and a body substring `WithStructuredOutputs`. Requests are built by `nativeOptionsRequest` (`native_options_test.go:503-555`), which resolves the `native` and `primary` providers with `ProviderName: "my-vllm.chat"` and the same type. Its fake backend returns 503 for `/primary/` paths (L518-522). It has nine callers: seven in `native_options_test.go` and two in `structured_output_test.go`.
- `ai-gateway/test/providerwire-v4/gateway-command.test.ts` runs the built command through the Go and TypeScript clients. Its "structured output candidate attribution" suite (L62-216) uses Anthropic and OpenAI fakes only. `FakeCompatible` (L3732) records each backend request body (L3772). The suite runs with `mise run test-ai-gateway-command` (`mise.toml:368`).

Upstream baseline: `@ai-sdk/openai-compatible` 3.0.71 (`test/conformance/upstream.yaml:12`), read with `git show @ai-sdk/openai-compatible@3.0.71:packages/openai-compatible/src/chat/openai-compatible-chat-language-model.ts`.

- L154: `supportsStructuredOutputs = config.supportsStructuredOutputs ?? false`. This is a construction-time setting for each provider.
- L244: `strictJsonSchema = compatibleOptions?.strictJsonSchema ?? true`. This is a per-call provider option.
- L250-262: a schema with the switch off produces an `unsupported` `responseFormat` warning. Its details text is `JSON response format schema is only supported with structuredOutputs`.
- L292-305: a schema with the switch on produces `{type:"json_schema", json_schema:{schema, strict, name: name ?? "response", description}}`. Otherwise the request uses `json_object`.
- The direct conformance golden uses `supportsStructuredOutputs: true` in both `test/conformance/tools/generate.mts:336` and `record.mts:571`, for fixture `test/conformance/openai-compatible/recorded/structured-json-output/`.

## Goals / Non-Goals

**Goals:**
- Give operators a way to set upstream's per-provider construction capability, with the same default and the same request shape.
- Keep existing catalogs unchanged in request behavior.
- Match upstream's off-path warning `details` text.

**Non-Goals:**
- Any change to the `providers/openai-compatible` response-format mapping or defaults. Only the off-path warning `details` text changes (Decision 4).
- A `strictJsonSchema` catalog key. Upstream reads it per call, and the Gateway already forwards per-call compatible options.
- Detecting the capability automatically or probing the backend.
- Exposing the capability through configured discovery. Discovery output is unchanged.
- Gateway conformance matrix harness changes. They are separate work.

## Decisions

1. **Key name and shape: `supportsStructuredOutputs`, a plain `bool` with `omitempty`.** The name is upstream's construction setting, spelled in the catalog's existing camelCase style (`apiKeyEnv`, `baseURL`, `providerName`). The field is copied into `ResolvedProvider` in `ResolveProviderSecrets`, and the catalog passes it to `openaicompatible.WithStructuredOutputs`.
   - Alternative: `*bool`, which would let validation reject any presence of the key on non-compatible types, including explicit `false`. Rejected. `providerName` accepts its zero value silently on every type. Explicit `false` there is harmless, so a pointer and nil checks would add complexity without benefit.
2. **Validation: only `true` is rejected on `anthropic` and `openai`.** The check goes in the existing `case "anthropic", "openai"` branch and uses the providerName message pattern: `config: providers.%s.supportsStructuredOutputs is only supported for openai-compatible providers`. Startup fails before readiness, like every other config error.
3. **Always pass the option.** The catalog calls `WithStructuredOutputs(providerConfig.SupportsStructuredOutputs)` unconditionally, the same way `WithProviderName` is passed. A `false` value equals the provider default, so existing catalogs are unchanged. Each fallback candidate reads its own provider's setting, because construction is per descriptor (`catalog.go:46-73`).
4. **Align the off-path warning with upstream.** `convertResponseFormat` (`convert_request.go:833-839`) uses upstream 3.0.71's `details`, `JSON response format schema is only supported with structuredOutputs`, keeping type `unsupported`, feature `responseFormat` and the `json_object` fallback. The provider gains a port of upstream's "should forward json response format as \"json_object\" and omit schema when structuredOutputs are disabled" test, unary and streaming, asserting the request body and the exact warning. The Gateway builds the provider from same-revision source (`ai-gateway/go.mod` replace), so one change keeps both modules green.
   - Alternative: keep the Go text and track the divergence separately. Rejected: this change exercises exactly this path, and fixing it here leaves no open divergence on it.
5. **Evidence: deterministic tests, no new fixtures.** No provider `input*.chunks.txt` is added or changed.
   - Service tests use real `BuildCatalog` construction with the fake HTTP backend, unary and streaming. The setting is chosen per resolved provider through an additive helper or option, so the existing `nativeOptionsRequest` callers keep their behavior.
   - Setting-on case across direct and fallback and the three name variants: `json_schema` shape, and `strict == true` by default because the body's `strictJsonSchema:false` is under the `openai` namespace, which the compatible model does not read.
   - Per-call override: with the setting on, `providerOptions.myVllm.strictJsonSchema:false` gives `strict == false`. The raw `my-vllm` key is not used because it adds a deprecation warning.
   - Mixed fallback: the failing primary (503 at `/primary`) and the serving fallback have opposite settings. Each recorded request's `response_format.type` matches its own provider's setting, and the output carries no `responseFormat` unsupported warning when the serving candidate has the setting on.
   - Warnings are asserted structurally (type `unsupported`, feature `responseFormat`), not by substring. The setting-off case asserts upstream's exact details text, which pins the aligned off path.
   - Client-level proof on main: `gateway-command.test.ts` adds a compatible provider with `supportsStructuredOutputs: true`, backed by `FakeCompatible`, and runs generate and stream through the Go and TypeScript clients. `FakeCompatible` returns fixed non-JSON text ("hello from fake compatible", L3804/L3814) and finishes streams only when the body contains the `normal-stream` marker (L3770, L3812). The test therefore adds an additive `structuredOutput` flag to `FakeCompatible` that returns `{"value":"ok"}` in place of both the unary content (L3804) and the stream delta (L3814), mirroring `FakeOpenAI.structuredOutput` (L3829, L3920), and uses the `normal-stream` prompt for streaming. The `json_schema` and no-warning assertions apply only to the low-level calls that carry the suite's schema response format (suite L92-96): the Go `generate`/`stream` capture modes and TypeScript `doGenerate`/`doStream`. For those calls it asserts the recorded `response_format.type` is `json_schema` and that no `responseFormat` unsupported warning is returned; a missing `warnings` field (Go `omitempty`) counts as empty. The suite's high-level calls (TypeScript `Output.json()` at L141/L165 and Go `structured-*` via `output.JSON(output.WithName(""))` at `providers/grafana/internal/capture/main.go:140,145`) send no schema (`output/json.go:30-36`), so their recorded backend requests are expected to be `json_object`; they only prove that `{"value":"ok"}` parses.

### Parity classification

- Setting default `false`, the `json_schema` shape, the name default only when absent, and `strict` defaulting to `true` per call: **parity-preserving**. The Go provider already matches upstream, and this change only makes the switch reachable.
- Expressing the setting as a YAML catalog key instead of a constructor argument: **parity-preserving Go adaptation**. The Gateway is a Go extension, and an operator catalog is its construction surface.
- The off-path warning `details` text differed from upstream (Go `JSON schema response format requires WithStructuredOutputs(true); using json_object.` vs upstream 3.0.71 `JSON response format schema is only supported with structuredOutputs`). The type and feature already matched. Classification: **implementation bug**, a pre-existing divergence in `providers/openai-compatible`, fixed here (Decision 4). No fixture covers the off path (the golden enables the setting), so the ported provider test is the regression evidence. `knownGaps` had no entry for it and stays unchanged.
- Issue #561 asked for the off-path warning to stay unchanged. The maintainer chose to align it with upstream instead, so the off path changes only in `details`.

### PARITY.md / knownGaps

No edit is expected. The `AI Gateway | Go extension | mixed` row (`PARITY.md:46`) already points to `openspec/specs/gateway-*` and `ai-gateway/` tests. Its status and evidence location do not change, and the `openai-compatible` adapter row is unaffected because the provider does not change. No accepted deviation or support boundary is added, so `knownGaps` stays as is. A metadata-only `mise run validate-parity-baseline` confirms the baseline still validates.

### Documentation

- Operators find provider keys in `ai-gateway/README.md` "Configure models" (L73-75). Add the key there, next to the existing `baseURL` sentence.
- `docs/providers/grafana-gateway.md` "Structured output" (L258-260) currently says compatible providers always use JSON mode with a warning. Qualify it with the new setting.
- `docs/providers/openai-compatible.md:44-50` already documents direct `WithStructuredOutputs(true)`, so no change is needed there.
- `docs/concepts/gateway-architecture.md` and `gateway-glossary.md` do not mention structured outputs or per-provider capabilities, and this change adds no flow, boundary or term to them. No drift is expected; confirm during implementation.
- The provisioning sample `ai-gateway/provisioning/aigateway/models.yaml` has only an Anthropic provider and stays unchanged.

## Risks / Trade-offs

- [An operator enables the setting for a backend that cannot handle `json_schema`] → The backend returns its own error through existing safe error handling. Docs say to enable the setting only after verifying the backend, matching the direct provider guidance.
- [Explicit `false` on a non-compatible type is accepted silently] → This is intentional and consistent with `providerName`. It has no effect on behavior.
- [A caller matches on the old warning `details` text] → `details` is human-readable text; type and feature are unchanged. The new text matches upstream, which TypeScript callers already see.

## Migration Plan

This change is additive. Existing catalogs need no edit and behave as before. To roll back, revert the source. A catalog that sets the key would then fail strict decoding, so remove the key before reverting.
