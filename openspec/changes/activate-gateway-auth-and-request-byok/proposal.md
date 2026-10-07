## Why

The prepared BYOK engine must be exposed without granting Cloud callers access to
configured accounts. Listener activation and account authorization must land
together, rather than briefly expose a trusted-header listener backed by the catalog.

## What Changes

- **BREAKING:** replace exclusive authentication modes and legacy listeners with private JWT, proxy-only Cloud and operational listeners in one process.
- Verify private JWTs for audience ai-sdk, concrete/wildcard namespace context and optional bound acting users, without a service allowlist.
- Assign immutable configured/BYOK access policy from verified provenance and enforce it for inference and discovery.
- Install request-local BYOK observers outside credential attempts with safe capture and bounded model metric labels.
- Prove both clients against the unified command and document the separate deployment isolation handoff.

## Capabilities

### New Capabilities

None; the BYOK execution capability is introduced by the preceding engine change.

### Modified Capabilities

- `ai-gateway-cloud-authentication`: simultaneous entry points, verified account policy and lifecycle.
- `gateway-model-catalog`: configured-access-only resolution.
- `gateway-configured-discovery`: configured-only projection and unsupported BYOK discovery.
- `gateway-provider-configuration`: exclude configured state from BYOK execution.
- `gateway-request-byok`: authenticated integration, server privacy and independent evidence.
- `gateway-text-observability`: request-scoped logical identity and bounded metrics.
- `grafana-gateway-client`: unified-command evidence, endpoint guidance and discovery behavior.
- `providerwire-v4-unary-runtime`: service error/discovery composition.
- `providerwire-v4-streaming-runtime`: request-scoped observation independent of native identity.

## Impact

Stack 3/3 for #317, depending on `add-gateway-request-byok-engine` and
`protect-gateway-client-byok-capture`. Owns auth/config/process/service wiring,
real-command tests, Docker/CI settings and operator/user guidance. Deployment-tools
and backend-enterprise are not edited. No live environment activation is claimed.
Extended credential/routing/evidence work remains separately owned.
