## Context

This slice layers on SDK readiness (#238) and provider-tool transport (#239). Source and tests use registered commit `ee3169b3c4880e2abe4d0d7c781243bb81822ec4`: Anthropic 4.0.65, Gateway 4.0.94, provider 4.0.18 and ai 7.0.116. The reference covers native MCP options, prompt conversion and producer result correlation, not Vercel's private hosted-service policy. The predecessor runtime already forwards native MCP configuration and supplied history; this slice adds consuming-boundary validation and response-derived continuation evidence.

## Goals / Non-Goals

Bound native Anthropic MCP configuration and validate consumed continuation without inventing Gateway execution, option inventories, metadata authority or fallback safety policy. Keep credentials, destinations, tenant authority, generic ownership and lifecycle protected. Do not add sessions, provenance frameworks, another stream reader or public SDK contracts.

## Decisions

- Resolve the same AnthropicOptions used by native conversion. Validate actual names, HTTPS destinations, optional credentials and tool configuration with named resource limits. Preserve missing/null/empty configuration and pointer presence. The existing Go API constructs URL definitions; its unconsumed input type and extension fields are not a transport inventory.
- Interpret continuation only at the native boundary. Consumed serverName uses the adapter's exact lookup; type uses its existing struct decoding. Generic pairing remains in the codec. Result metadata need not repeat the originating call's fields.
- Transport metadata opaquely in both modes and independent clients. A semantic-looking caller, item or type field is not authority. Do not enforce configured-name membership on responses, project fields or infer the selected candidate from mutable context.
- Keep ordinary fallback commitment: unary success or any first provider part selects. Eligible earlier failures may advance even when remote effects or billing already occurred. Developers choose idempotency/deduplication or avoid unsafe fallback; no exactly-once claim or new safety flag.
- Preserve independently controlled capture. Developer-returned metadata is not permission for metadata-only telemetry to retain it.
- Retain separate pinned-client request goldens, synthetic deferred transport and authenticated native-fake response-derived continuation. Inputs under provider recorded/upstream directories remain untouched.

## Risks / Validation Limits

Anthropic initiates hosted MCP egress. Deterministic fakes do not establish live remote effects, network policy, deployed readiness or standalone published-module adoption. Matching names do not bind endpoint/token/provider across requests; validation is stateless and request-local. An unobserved attempt can create a ticket before losing its response, and another candidate can create a duplicate.

## Migration

Each slice must pass on its immediate parent using candidate source and merged immutable module pins. No parent-owner branches change. Production rollout requires separate operational and module/image publication checks.
