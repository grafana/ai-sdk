# Proposal

## Why

Every installed backend plugin also starts inside a Grafana Cloud stack's own Grafana, which provides no platform signing keys and no Gateway configuration; only the plugin operator does. The Gateway exits there ("config: set GRAFANA_JWKS_URL or GRAFANA_JWKS to verify platform tokens"), so the stack marks the plugin as failed and its app pages do not render (#661). Other App Platform plugins start in the stack and stay idle, and the stack's copy serves nothing the Gateway needs: the frontend only calls the Gateway API, which the platform routes to the operator.

## What Changes

- Without a platform signing-key source, the Gateway starts idle instead of failing: it reads no other setting, catalog or secret, constructs no catalog, provider clients, telemetry or handlers, and the plugin host rejects every route request because the Gateway configures no authenticator.
- The plugin logs a `plugin_idle` lifecycle event; the plugin host SDK logs its standard warning that plugin protocol v3 requests will be rejected.
- Requesting insecure authentication skipping without signing keys still fails startup, so the Gateway keeps no unverified mode.
- With a signing-key source, startup is unchanged: invalid signing keys and invalid configuration still fail startup before any request is served.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-caller-access`: a missing signing-key source starts the Gateway idle instead of failing startup.
- `gateway-plugin-host`: configuration is validated and health reported only for a Gateway with signing keys; an idle Gateway reports healthy.

## Impact

- **Code:** `ai-gateway/pkg/internal/gateway` (`New` resolves signing keys first and returns an idle Gateway without them) and `ai-gateway/pkg/main.go` (idle lifecycle event).
- **Tests:** gateway unit tests for idle startup, and a plugin-process test that launches the plugin without signing keys through the test platform host, which gains an option to omit them.
- **Docs:** `ai-gateway/README.md` startup behavior.
