## Why

Provider-tool transport needs native Anthropic server configuration and consumed continuation to support hosted MCP. The original design's direct-only gates and metadata inventories are unnecessary under the revised Gateway principle.

## What Changes

- Enable bounded resolved MCP configuration at native Anthropic consumption, including configured fallback attempts.
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
