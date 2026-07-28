## Context

The Bedrock provider already serializes explicit adaptive thinking and effort, but root reasoning derivation selected budget-token thinking for every Anthropic model. The Bedrock module cannot reuse the Anthropic provider implementation because provider modules are dependency-isolated.

## Goals / Non-Goals

**Goals:**

- Match the registered Bedrock package's capability-gated reasoning behavior.
- Keep adaptive capability detection compatible with cross-region Bedrock model IDs.
- Preserve existing behavior for older and unknown Anthropic models.

**Non-Goals:**

- Generalize Bedrock capabilities beyond the reasoning fields needed by this change.
- Upgrade the registered upstream baseline.

## Decisions

- Keep a Bedrock-local reasoning capability lookup using the exact model-family substrings and maximum output tokens from the registered upstream baseline. Importing the Anthropic provider would violate module isolation, while separate adaptive and budget tables could drift.
- Derive root reasoning first, then overlay non-zero explicit provider reasoning fields. This matches upstream object-spread precedence within Go's zero-value constraints and allows partial configs to retain derived fields.
- Serialize merged non-adaptive type and enabled budget fields alongside derived effort for Nova-style non-Anthropic reasoning configs, while retaining upstream warnings for raw budget and adaptive options.
- Replace Anthropic reasoning with disabled configuration for root `none`, and clear budget and effort whenever the merged type is disabled.
- Cover capability branches and partial-provider-option merging with provider request snapshots: Sonnet 4.6 requires adaptive thinking, display, and effort, while the existing Sonnet 4.5 snapshot retains budget-token thinking.

## Risks / Trade-offs

- The local model-family set can drift when the upstream baseline changes. Parity upgrades must review and update the capability lookup and tests together.
- Go zero values cannot distinguish omitted provider fields from explicitly supplied empty values. In particular, the current `int` field treats an explicit zero budget as absent even though the upstream schema accepts zero; supported operational thinking budgets are normally positive, so this remains a narrow Go adaptation.
