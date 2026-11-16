# Design

## Context

See proposal.md for motivation. The registered reference is `@ai-sdk/amazon-bedrock` 5.0.118, commit `b7f6255acaae7724adc0fd37aac3dae3f3466578`, specifically `resolveAmazonBedrockReasoningConfig` and its request/warning tests in `packages/amazon-bedrock/src/amazon-bedrock-chat-language-model{,.test}.ts`.

Go currently derives first, merges second and clears disabled controls last. Its typed options can retain a shared reasoning-config pointer through provider-default resolution; downstream between-tools effort capping can mutate it. This is an implementation bug within the existing provider-adapter surface (`PARITY.md`: mixed).

## Goals / Non-Goals

Match precedence, warnings and ownership without changing the public struct or namespace reader. Keep existing type, display, explicit zero presence and model routing. Do not alter the established Nova high-effort token omission or portable max mappings.

## Decisions

- Clone explicit configuration before any request preparation. The scalar-only config needs a value copy, not JSON round-tripping or a public API change.
- Return explicit disabled configuration with budget and effort cleared for custom reasoning before any derivation. Preserve provider-default controls, matching upstream's earlier default-return branch.
- Select the relevant explicit control before deriving: budget for non-adaptive Anthropic, effort for adaptive Anthropic and non-Anthropic. Fill type defaults independently. Keep derivation helpers for mappings actually used. Filtering warnings after derivation was rejected because it obscures which values were actually resolved.
- Retain raw zero presence tracked by the existing option decoder. Typed zero still means omitted, a documented Go adaptation.
- Preserve the existing Sonnet 5.5 none-to-between-tools deviation only when explicit disabled configuration did not already decide the result.
- Add synthetic request/HTTP tests paired with warnings, plus concurrent reuse under the race detector. No synthetic input is added to recorded/upstream fixture directories. Existing recorded request expectations will be replayed; no regeneration is anticipated because those inputs do not cover the override regression.

## Risks / Trade-offs

- Caller reuse could expose hidden aliasing → exercise raw and typed options and provider-default between-tools capping concurrently.
- Removing all warnings could hide valid diagnostics → retain tests for derived mappings, non-Anthropic budgets and Nova token omission.
- Mock HTTP acceptance does not establish AWS acceptance → report this limit and avoid a live-provider claim.

## Migration Plan

No migration or dependency change. Revert the resolver change to roll back; regressions remain captured by focused tests.
