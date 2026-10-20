# Proposal

## Why

The frozen mature upstream reference adds observable LanguageModelV4 fields and changes native source emission. Moving pins without carrying these fields through the Go SDK, frontend and Gateway would fail existing conformance and supported continuation flows.

## What Changes

- Advance the coherent reference and consumer lockfile using the repository-selected record.
- Preserve text citations and text-encoded tool-input deltas across native adapters, orchestration, persistence and both Gateway clients.
- Emit Anthropic message-start custom metadata, align retrieval-source ordering, and avoid invalid stored OpenAI text references after reasoning removal.
- Carry portable maximum reasoning through supported mappings and preserve unavailable-tool history normalization.
- Keep optional parity packages and unresolved issue dispositions outside implementation scope.

## Capabilities

### New Capabilities

- `text-citations`: generated text references, retrieval-source separation and end-to-end transport.

### Modified Capabilities

- `sdk-provider-tools`: text-encoded partial input and persistence.
- `native-custom-content`: native Anthropic message-start parts.
- `effort-level`: portable maximum reasoning.
- `ui-tool-state-persistence`: unavailable static-tool output normalization.
- `openai-responses-provider`: stored text reference eligibility and deduplication.

## Impact

Root SDK, Anthropic/OpenAI/Bedrock/Grafana modules, Gateway schemas/runtime and TypeScript harnesses. Existing public module pins remain real and unchanged; coordinated candidate-source checks establish integration, not independent publication readiness. Provider recordings remain unchanged.
