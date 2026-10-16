## Why

PR #248 freezes temporary Gateway restrictions and discards native response identity. Align it with the delivered native-option/fallback contract and the approved Chat-specific warnings and streaming decisions.

## What Changes

- Replace model allowlists and fallback refusals with candidate-local native default translation.
- Validate a bounded explicit Chat schema and typed DTOs without reflective JSON walking.
- Preserve native identity, stable stream identity and bounded Grafana warnings/late identity extensions.
- Exercise real-command heterogeneous fallback with official OpenAI Go/JavaScript clients and update the adapter contract.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-openai-chat-completions-adapter`: mapped fallback, native defaults, explicit request validation, response identity and diagnostics.

## Impact

AGPL Chat adapter and command composition, official SDK contracts, docs and parity evidence. ProviderWire wire format and SDK provider implementations are unchanged. The current main already contains #326/#328/#332/#238; unrelated BYOK, hosted tools and execution-evidence stacks are not prerequisites.
