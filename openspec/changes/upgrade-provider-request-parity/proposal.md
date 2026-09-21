## Why

Phase 1 of `~/src/ai-sdk/test/conformance/UPGRADE_PLAN.md` makes existing provider call settings produce valid requests for the selected model. Current Go conversion can select future-Claude defaults for dated Vertex Claude 4 IDs, misrepresent OpenAI allowed tools, and send invalid Bedrock document names or strict schemas; the assessed request delta also includes OpenAI schema normalization and Bedrock OpenAI reasoning routing.

## What Changes

- Recognize dated Vertex Sonnet 4 and Opus 4 IDs with their actual capabilities and 64k/32k output limits.
- Resolve OpenAI Responses `allowedTools` from prepared declarations: function/custom names, built-in types, and MCP server labels; preserve ordered selections and upstream ambiguity, warning, and empty-selection error behavior.
- Sanitize Bedrock document names identically for user input and tool-result documents, with deterministic fallback names.
- Omit incompatible Bedrock `strict: true` with a warning, inspecting nested schemas without rewriting them; preserve valid strict controls and model-specific rejection.
- Normalize OpenAI string `propertyNames` constraints for existing response/tool schemas with compatibility warnings, rejecting unsupported forms without mutating caller schemas.
- Correct existing Bedrock reasoning settings for native/cross-region OpenAI models, retaining flat GPT-OSS effort and using nested effort for other OpenAI models.
- Infer Anthropic application-inference-profile requests from explicit reasoning-budget presence across request construction, without a public pointer migration.
- Default Bedrock Sonnet 4.6 and Haiku 4.5 structured output to the existing JSON-tool route independently of strict-tool support.
- Forward the existing `anthropic.disableParallelToolUse` setting through Bedrock, including synthetic JSON tools, with one correct tool-choice representation.
- Implement these nine corrections approved by Nara in the revised upgrade plan. New model-family, explicit output-mode, thinking-update/binding, and service-tier APIs remain planner-owned pending decisions, not accepted gaps and not prerequisites unless a concrete dependency is found.
- Own focused regressions and early exact-target request evidence here; retain the registered baseline and leave canonical pins, lockfile, and target snapshot certification to phase 7.

## Capabilities

### New Capabilities

None; this change corrects existing provider request behavior.

### Modified Capabilities

- `model-capabilities`: recognize dated Vertex Claude 4 IDs before future-Claude fallback and apply the correct request defaults.
- `openai-responses-provider`: declaration-aware allowed-tool resolution and non-mutating request-schema normalization, including use through the shared Mantle adapter.
- `bedrock-provider`: document-name sanitation, strict-schema compatibility, OpenAI reasoning shapes, budget-based profile inference, default JSON-tool routing, and existing parallel-tool option forwarding.

## Impact

Primary code: `providers/anthropic/models.go` and request tests; `providers/openai/prepare_tools.go`, `convert_request.go`, and their tests; `providers/bedrock/convert_messages.go`, `convert_tools.go`, `model_family.go`, `convert_request.go`, and their tests. Shared OpenAI changes require Mantle regression coverage. Provider implementation/request mapping is the affected parity layer; warnings remain partly unit/source-reviewed coverage.

References are the registered Anthropic 4.0.38, OpenAI 4.0.41, and Bedrock 5.0.55 packages versus fixed target 4.0.57, 4.0.70, and 5.0.87 at `b3033f77f0459bcc68df182c8052f52305467bcc`. No baseline upgrade, new provider family, generic provider-interface migration, response/history conversion, local tool-choice enforcement, UI wire change, discovery, or recovery scheduling is included. No new public API or dependency is planned. New schema rejection and empty allow-list errors are deliberate target validation changes, not backward-compatibility modes.
