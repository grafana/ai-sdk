## Why

Converse requests currently infer Anthropic/OpenAI families incompletely and choose structured-output and reasoning fields inconsistent with the registered `@ai-sdk/amazon-bedrock` 5.0.88 baseline (`08ae5ad05bc12496dd1ffcf64e34419e0831300d`). Application inference profiles, model-specific JSON/strict support, and parallel-tool routing can produce unsupported or conflicting Bedrock requests (#222).

## What Changes

- Propose an explicit Anthropic model-family constructor setting (subject to OpenSpec/owner API approval before implementation) and recognize application inference-profile ARNs with a configured reasoning budget; preserve root reasoning resolution for dated, regional, known legacy and unknown Claude/non-Claude IDs.
- Select `auto`, `jsonTool`, or `outputFormat` structured output from Bedrock options, with Anthropic option fallback, respecting model-specific native/strict gates, thinking, user tools, and existing JSON-tool response translation.
- Align Anthropic provider-tool/strict-schema warnings, effective parallel-tool choice, and pass-through field/beta precedence; distinguish GPT-OSS flat effort from newer OpenAI nested effort.
- Sanitize document names before Converse requests; cover request and response failure modes with focused tests without creating synthetic recorded provider events.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `bedrock-provider`: Change Converse model-family inference, structured-output/strict-tool/parallel-tool routing, reasoning fields, document names, and option precedence.

## Impact

`providers/bedrock/` constructor/options, model-family capability resolution, Converse conversion and focused tests; existing `openspec/specs/bedrock-provider/spec.md` contract. Schema sanitation already imports `github.com/grafana/ai-sdk/internal/anthropicschema`; any new root schema helper must first be publicly published and proven resolvable by the standalone Bedrock consumer, or remain local to Bedrock. No Mantle or Anthropic Invoke changes. No recorded conformance inputs are fabricated; existing recorded/upstream snapshots are refreshed only if provenance is retained.
