## 1. Catalog setting (config package)

- [x] 1.1 Write failing tests in `ai-gateway/cmd/grafana-ai-gateway/internal/config/file_test.go`. Verify they fail before 1.2 with `(cd ai-gateway && go test ./cmd/grafana-ai-gateway/internal/config/)`.
  - `TestLoadFile_OpenAICompatibleProvider`, or a sibling test: `supportsStructuredOutputs: true` on a compatible provider loads, and the resolved provider carries it.
  - When the key is omitted, the resolved provider carries `false`.
  - `TestLoadFile_ProviderErrorsNameTheField`: add `anthropic` and `openai` cases with `supportsStructuredOutputs: true`, each wanting `providers.anthropic-primary.supportsStructuredOutputs`.
  - `TestLoadFile_StrictBoundedDocument`: add the same two rejection rows.
  - Add an accept case for explicit `false` on `anthropic`/`openai`.
- [x] 1.2 Make the tests from 1.1 pass. Verify that `TestLoadFile_*` and `TestResolveProviderSecrets` pass.
  - Add `SupportsStructuredOutputs bool` with `yaml:"supportsStructuredOutputs,omitempty"` to `Provider` (`file.go:33-39`) and to `ResolvedProvider` (`file.go:57-62`).
  - Copy the field across in `ResolveProviderSecrets` (`file.go:253`).
  - In `Validate`'s `case "anthropic", "openai"` branch (`file.go:136-139`), reject `true` with `config: providers.%s.supportsStructuredOutputs is only supported for openai-compatible providers`.

## 2. Off-path warning parity (openai-compatible provider)

- [x] 2.1 Port upstream 3.0.71's "should forward json response format as \"json_object\" and omit schema when structuredOutputs are disabled" test to `providers/openai-compatible/structured_output_test.go`, unary and streaming. Assert the request carries `response_format: {type: "json_object"}` without a schema and the warnings equal one `unsupported` `responseFormat` warning with details `JSON response format schema is only supported with structuredOutputs`. Verify it fails with `(cd providers/openai-compatible && go test -run TestStructuredOutput ./...)`.
- [x] 2.2 In `convertResponseFormat` (`providers/openai-compatible/convert_request.go:833-839`), change only the `details` text to upstream's. Verify `(cd providers/openai-compatible && go test ./...)` passes.

## 3. Compatible model construction (service package)

- [x] 3.1 Write failing tests in `ai-gateway/cmd/grafana-ai-gateway/internal/service/structured_output_test.go`. Verify the new cases fail before 3.2 with `(cd ai-gateway && go test -run TestStructuredOutput ./cmd/grafana-ai-gateway/internal/service/)`.
  - Make the structured-output setting selectable per resolved provider (`native` and `primary`) through an additive helper or option next to `nativeOptionsRequest` (`native_options_test.go:503-555`). Its nine existing callers (seven in `native_options_test.go`, two in `structured_output_test.go`) keep their signature and behavior.
  - Setting-on case, both providers on, across stream × fallback × name. Assert `response_format.type == "json_schema"` and `json_schema.name` (`response` when absent, `""` or `answer` when given). Also assert `description == "description"`, `strict == true` (the body's `strictJsonSchema` is under the `openai` namespace, which compatible does not read), `schema.type == "object"`, and that the output has no `responseFormat` unsupported warning.
  - Per-call strict override, setting on, unary and streaming: a body with `providerOptions.myVllm.strictJsonSchema:false` gives `json_schema.strict == false`. Do not use the raw `my-vllm` or `openai-compatible` keys, which add a deprecation warning (`providers/openai-compatible/convert_request.go:230-240`).
  - Mixed fallback, unary and streaming: the primary fails (503 at `/primary`) before producing output, and it and the serving fallback have opposite settings. Assert each recorded request's `response_format.type` matches its own provider's setting (`json_object` for off, `json_schema` for on), and that the output has no `responseFormat` unsupported warning when the serving candidate has the setting on.
  - Assert warnings structurally (type `unsupported`, feature `responseFormat`) instead of the `WithStructuredOutputs` substring.
  - Keep the existing `openai-compatible` case as the setting-off case. Keep `json_object`, switch its warning check to the structural form, and assert upstream's details text `JSON response format schema is only supported with structuredOutputs`.
- [x] 3.2 In `service/catalog.go:66-73`, add `openaicompatible.WithStructuredOutputs(providerConfig.SupportsStructuredOutputs)` to the compatible construction. Verify with `(cd ai-gateway && go test ./cmd/grafana-ai-gateway/internal/service/)`: the on, override, mixed-fallback and off cases pass, and the other catalog and native-option tests are unchanged.
- [x] 3.3 Client-level proof: in `ai-gateway/test/providerwire-v4/gateway-command.test.ts`, extend the "structured output candidate attribution" suite (L62-216) with a compatible provider configured `supportsStructuredOutputs: true` and backed by the existing `FakeCompatible` (L3732), which records request bodies. `FakeCompatible` returns fixed non-JSON text ("hello from fake compatible", L3804/L3814) and finishes streams only when the body contains the `normal-stream` marker (L3770, L3812). Add an additive `structuredOutput` flag to `FakeCompatible` that returns `{"value":"ok"}` in place of both the unary content (L3804) and the stream delta (L3814), mirroring `FakeOpenAI.structuredOutput` (L3829, L3920), and use the `normal-stream` prompt for streaming. Run generate and stream through both the Go and TypeScript clients. The schema assertions apply only to the low-level calls that carry the suite's schema response format (suite L92-96): the Go `generate`/`stream` capture modes and TypeScript `doGenerate`/`doStream`. For those calls, assert each recorded backend request has `response_format.type === "json_schema"` and that no `responseFormat` unsupported warning is returned; a missing `warnings` field (Go `omitempty`) counts as empty. The suite's high-level calls (TypeScript `Output.json()` at L141/L165 and Go `structured-*` via `output.JSON(output.WithName(""))` at `providers/grafana/internal/capture/main.go:140,145`) send no schema (`output/json.go:30-36`), so their recorded backend requests are expected to be `json_object`; they only prove that `{"value":"ok"}` parses. Verify with `mise run test-ai-gateway-command`.

## 4. Documentation

- [x] 4.1 Document `supportsStructuredOutputs` in `ai-gateway/README.md` "Configure models" (L73-75), next to the `baseURL` sentence. Say that it is only for `openai-compatible`, defaults to `false`, sends JSON-schema response formats, and should be enabled only for backends verified to support them. Verify that the README renders and stays within its current section.
- [x] 4.2 In `docs/providers/grafana-gateway.md` "Structured output" (L258-260), qualify the sentence that says compatible providers always use JSON mode with a warning. With `supportsStructuredOutputs` they follow the schema, and they keep JSON mode with a warning without it. Verify the page links remain valid.
- [ ] 4.3 Confirm that `docs/concepts/gateway-architecture.md` and `docs/concepts/gateway-glossary.md` need no change. Neither describes structured outputs or per-provider capabilities, and no flow, boundary or term changes. Record the result in the PR description, or update them if drift is found.
- [x] 4.4 Confirm no docs or godoc quote the old off-path warning text (`grep -rn "requires WithStructuredOutputs(true)"` outside `openspec/` returns nothing).

## 5. Integration validation

- [x] 5.1 Run `mise run fmt`, `mise run vet` and `mise run lint`. Verify all pass with no diff from fmt.
- [x] 5.2 Run `mise run test-ai-gateway`, `mise run test-ai-gateway-command`, then `mise run test`. Verify all pass.
- [x] 5.3 Run `mise run validate-parity-baseline`. Verify it passes, and confirm `test/conformance/PARITY.md` and `upstream.yaml` `knownGaps` are unchanged (see design, PARITY.md / knownGaps).
- [x] 5.4 Run `openspec validate add-gateway-compatible-structured-outputs --strict`. Verify it passes.
