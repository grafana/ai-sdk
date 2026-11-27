## Why

Core deferred-tool search uses a fixed keyword ranker and caps every search at
five results. Applications cannot choose a different result count or use
their own domain-specific ranking while preserving the SDK's discovery rules.

## What Changes

- Add optional per-tool result limits and a ranked-name search callback.
- Validate the limit before model I/O; ignore duplicates and ineligible names
  before applying the limit. A failed or canceled search discovers nothing.
- Preserve default search behavior, caller boundaries and next-step activation.

## Capabilities

### Modified Capabilities

- `deferred-tool-discovery`: configurable ranking and match limits.

## Impact

Core ToolSearch API, guide, and provider-independent request and UI fixtures. No
provider adapter or UI protocol changes.
