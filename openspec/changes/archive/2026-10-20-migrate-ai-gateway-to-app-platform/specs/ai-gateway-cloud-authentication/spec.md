# Spec Delta

## REMOVED Requirements

### Requirement: Unified dependency construction
**Reason**: The standalone command and its JWT dependency graph are removed.
**Migration**: Configured and BYOK dependencies are constructed once at plugin startup under `gateway-plugin-host`; BYOK isolation from configured state remains in `gateway-request-byok` and `gateway-provider-configuration`.

### Requirement: Simultaneous isolated authentication entry points
**Reason**: Gateway listeners and Gateway-side JWT verification are removed.
**Migration**: Use `gateway-caller-access`; account access derives from the platform-verified delegated identity.

### Requirement: Request-level account access and namespace context
**Reason**: Policy no longer follows listener provenance or JWT namespaces.
**Migration**: Use `gateway-caller-access`; account access derives from the platform-verified delegated identity.

### Requirement: Coordinated multi-listener lifecycle
**Reason**: The Gateway binds no listeners.
**Migration**: Use `gateway-plugin-host`; the plugin host owns admission, routing, transport and lifecycle.

### Requirement: Trusted proxy contract
**Reason**: The trusted-proxy Cloud ingress is removed; the platform authenticates callers.
**Migration**: Use `gateway-caller-access`; account access derives from the platform-verified delegated identity.

### Requirement: Header-only authentication boundary
**Reason**: The platform authenticates before the plugin receives the request.
**Migration**: Use `gateway-caller-access`; account access derives from the platform-verified delegated identity.

### Requirement: Distinct trusted Cloud identity
**Reason**: `X-Scope-OrgID` assertions are no longer an identity source.
**Migration**: Use `gateway-caller-access`; account access derives from the platform-verified delegated identity.

### Requirement: Cloud stack assertion validation
**Reason**: `X-Scope-OrgID` assertions are no longer an identity source.
**Migration**: Use `gateway-caller-access`; account access derives from the platform-verified delegated identity.

### Requirement: Cloud Caller identity fields
**Reason**: Cloud caller fields derived from proxy headers are removed.
**Migration**: Use `gateway-caller-access`; account access derives from the platform-verified delegated identity.

### Requirement: Authentication-mode write timeout accounting
**Reason**: Gateway HTTP servers and JWKS latency accounting are removed.
**Migration**: Use `gateway-plugin-host`; the plugin host owns admission, routing, transport and lifecycle.

### Requirement: Separate API and operational dispatch
**Reason**: Operational routes and Gateway-owned dispatch paths are removed.
**Migration**: Use `gateway-plugin-host`; the plugin host owns admission, routing, transport and lifecycle.

### Requirement: Shared listener readiness and shutdown
**Reason**: The host owns readiness and shutdown.
**Migration**: Use the host-owned lifecycle and health requirements in `gateway-plugin-host`.

### Requirement: Client composition and credential privacy
**Reason**: Real-command and dummy-edge evidence is replaced.
**Migration**: Use the protocol test host evidence in `gateway-plugin-host`.

### Requirement: Cloud low-level output-token defaults
**Reason**: Cloud-edge-specific evidence is replaced.
**Migration**: Use the protocol test host evidence in `gateway-plugin-host`.

### Requirement: Cloud high-level text-only client composition
**Reason**: Cloud-edge-specific evidence is replaced.
**Migration**: Use the protocol test host evidence in `gateway-plugin-host`.

### Requirement: Unrewritten SDK choice and body headers
**Reason**: Shim-specific evidence is replaced.
**Migration**: Use the protocol test host evidence in `gateway-plugin-host`.

### Requirement: Cloud authentication diagnostic privacy
**Reason**: Gateway-side authentication diagnostics are removed.
**Migration**: Use `gateway-caller-access` verified caller attribution and `gateway-plugin-telemetry` access decision metrics.

### Requirement: Streaming flush through authentication middleware
**Reason**: Gateway authentication middleware is removed.
**Migration**: Use incremental streaming in `gateway-plugin-host`.
