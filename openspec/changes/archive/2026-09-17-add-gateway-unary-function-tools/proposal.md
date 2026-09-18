## Why

WP11 (#105) lets authenticated clients define function tools, receive unary tool calls, and return selected tool results through the strict Gateway. The integrated WP7 client and WP8 logical observability must be part of this capability, so Gateway-only coverage is insufficient.

## What Changes

- Enable unary function definitions, choices, assistant tool-call history and tool-role selected results through explicit strict mapping.
- Preserve required empty values, opaque JSON, and presence-aware Strict; normalize absent/empty descriptions.
- Extend the existing private unary encoder, independent Go client, direct provider validation/conversion, and metadata-only logical observation.
- Keep streaming tools unsupported until WP12 and tools on fallback-configured routes unsupported until replay/idempotency is specified.
- Prove equivalent unary round trips with the exact registered Vercel and Go clients.

## Capabilities

### New Capabilities

- `gateway-unary-function-tools`: Function definitions, selected result arms, direct-route unary round trips, provider conversion and logical observation.

### Modified Capabilities

- `providerwire-v4-unary-runtime`: Enable this unary subset while preserving complete validation and bounded private output.

## Impact

Apache: provider domain/validation and affected native converters, existing `providers/grafana`, and reusable Agent Observability mappings only where evidence finds a gap. AGPL: `ai-gateway/providerwire/v4`, service composition/observation, contract workspace and real-handler tests. Depends on WP7 and WP8 (integrated heads `e0e6c01`, `a2177d9`); independent of WP9 delivery but must preserve its route guard if integrated. No production deployment is authorized by this planning change.
