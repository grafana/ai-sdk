# Proposal

## Why

Anthropic can return an authenticated compaction summary for continuation, but the Go provider drops the signature and has no way to request the summary on demand. Replaying an unsigned block in place of a signed one loses the information needed to authenticate the summary.

## What Changes

- Add an opt-in on-demand compaction option, its required beta, and preflight validation for incompatible provider options.
- Preserve the summary and signature in unary and streaming text metadata and on the next native request. Keep legacy unsigned replay and empty-summary omission.

## Capabilities

### New Capabilities

- `anthropic-compaction`: on-demand compaction request and response lifecycle.

### Modified Capabilities

- `anthropic-prompt-conversion`: replay signed compaction blocks with the corresponding beta.

## Impact

The Anthropic provider and focused tests change. No new dependency, core content type, context editing, Bedrock support, or Gateway API, filtering, routing, or certification.
