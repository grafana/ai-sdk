# Spec Delta

## REMOVED Requirements

### Requirement: Explicit boot authentication configuration
**Reason**: Standalone self-hosting with static keys is removed; self-hosted deployments run the plugin inside a self-hosted Grafana.
**Migration**: Remove `auth` and `server` blocks; `gateway-plugin-host` rejects them.

### Requirement: Selected authentication validation
**Reason**: Standalone self-hosting with static keys is removed; self-hosted deployments run the plugin inside a self-hosted Grafana.
**Migration**: Use `gateway-caller-access`; account access derives from the platform-verified delegated identity.

### Requirement: Static credential snapshots
**Reason**: Standalone self-hosting with static keys is removed; self-hosted deployments run the plugin inside a self-hosted Grafana.
**Migration**: Use Grafana service account tokens or access tokens accepted by the platform.

### Requirement: Fail-closed local admission
**Reason**: Standalone self-hosting with static keys is removed; self-hosted deployments run the plugin inside a self-hosted Grafana.
**Migration**: Use `gateway-caller-access`; account access derives from the platform-verified delegated identity.

### Requirement: Local identity and policy
**Reason**: Standalone self-hosting with static keys is removed; self-hosted deployments run the plugin inside a self-hosted Grafana.
**Migration**: Use `gateway-caller-access`; account access derives from the platform-verified delegated identity.

### Requirement: Standalone production composition
**Reason**: Standalone self-hosting with static keys is removed; self-hosted deployments run the plugin inside a self-hosted Grafana.
**Migration**: Use `gateway-plugin-host`; the plugin host owns admission, routing, transport and lifecycle.

### Requirement: Credential-safe operations and evidence
**Reason**: Standalone self-hosting with static keys is removed; self-hosted deployments run the plugin inside a self-hosted Grafana.
**Migration**: Use `gateway-caller-access` verified caller attribution and the credential scenarios in `gateway-plugin-host`.

### Requirement: Production image evidence
**Reason**: The container image is removed.
**Migration**: Use `gateway-plugin-packaging` validation.

### Requirement: Temporary hashing buffer cleanup
**Reason**: Standalone self-hosting with static keys is removed; self-hosted deployments run the plugin inside a self-hosted Grafana.
**Migration**: No Gateway-held static credentials remain.
