## Why

The provider-tool runtime already forwards native Anthropic MCP configuration and supplied history. This change adds request-local resource and destination bounds, validates server names only when native Anthropic consumes continuation, and proves response-derived continuation without restoring capability gates or metadata inventories.

## What Changes

- Bound resolved MCP configuration at native Anthropic consumption, including configured fallback attempts.
- Validate consumed continuation names while preserving generic ownership, ID/name and lifecycle protections.
- Preserve opaque response and foreign-option transport without metadata classification, projection, configured-response membership or policy intersections.
- Retain current fallback mechanics and document unknown-outcome billing/effect duplication.
- Prove native projection, independent clients, response-derived continuation and separately controlled telemetry capture.

## Capabilities

### New Capabilities

- `gateway-anthropic-mcp`: native hosted-MCP configuration and continuation with bounded transport and independent acceptance.

### Modified Capabilities

None. Generic provider-tool and fallback contracts remain unchanged.

## Impact

Depends on #238 and #239. Changes AGPL native option validation and tests/docs; Apache modules remain independent. Registered baseline: `ee3169b3c4880e2abe4d0d7c781243bb81822ec4` (Anthropic 4.0.65, Gateway 4.0.94, provider 4.0.18, ai 7.0.116). No upstream pins or authentic provider recordings change. Live MCP egress, deployment and published-module adoption remain unverified by deterministic candidate-source checks.
