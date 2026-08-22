## REMOVED Requirements

### Requirement: Module location and naming
**Reason**: The legacy `providers/grafana` nested module is removed.
**Migration**: Consumers may use the client source at repository tag `v0.1.0-alpha.1` or in Git history; no independently fetchable nested-module version is claimed.

### Requirement: LanguageModel interface implementation
**Reason**: The only implementation in this module consumes the retired transport.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history until the strict-only client is introduced.

### Requirement: Constructor naming reflects cloud-only auth
**Reason**: The constructor belongs to the deleted legacy module.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history for cloud-auth behavior.

### Requirement: Pre-minted access token constructor
**Reason**: The constructor belongs to the deleted legacy module.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history for access-token behavior.

### Requirement: Cloud auth configuration
**Reason**: This configuration is owned by the deleted legacy module.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history; later strict-client work will reconsider the ergonomics from Git history.

### Requirement: Access token configuration
**Reason**: This configuration is owned by the deleted legacy module.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history; later strict-client work will reconsider the ergonomics from Git history.

### Requirement: Token exchange uses authlib
**Reason**: Token exchange is implemented only by the deleted legacy client.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history for this authentication path.

### Requirement: Access token mode does not perform CAP exchange
**Reason**: This behavior is implemented only by the deleted legacy client.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history for this authentication path.

### Requirement: Optional user ID token forwarding
**Reason**: This context helper and forwarding behavior belong to the deleted legacy client.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history until the strict client reintroduces acting-user behavior.

### Requirement: Gateway-style provider-wire HTTP contract
**Reason**: The requirement defines the retired tolerant transport.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history; no transport replacement is provided here.

### Requirement: Streaming response handling
**Reason**: The response reader consumes the retired SSE codec.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history until the strict bounded Go client exists.

### Requirement: Non-streaming response handling
**Reason**: The response reader consumes the retired unary codec.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history until the strict bounded Go client exists.

### Requirement: Error reconstruction preserves retry semantics
**Reason**: Grafana-specific reconstruction and normalization are removed with the legacy client.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history; the future strict client will decode safe public errors independently.

### Requirement: Streaming channel buffering
**Reason**: This implementation detail belongs to the deleted model client.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history for the historical behavior.

### Requirement: Registry integration
**Reason**: This provider implementation is removed, while the registry itself remains supported.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history if Grafana provider registration is required.

### Requirement: Identity reporting
**Reason**: This model implementation is removed.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history for the historical identity behavior.

### Requirement: Integration tests against a fake provider-wire hosted endpoint
**Reason**: These tests verify only the retired client and protocol.
**Migration**: Use Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history for historical maintenance; new strict-client tests will be introduced later.

### Requirement: Conformance suite reuses Anthropic fixtures to prove transparent-transport equivalence
**Reason**: The repository no longer claims transparent transport equivalence for the deleted module.
**Migration**: Retain direct Anthropic conformance; strict Gateway evidence is deferred to the strict contract work package.

### Requirement: Provider relies on root provider-wire schema coverage
**Reason**: Both the legacy provider and root wire schema helpers are removed.
**Migration**: Pin root `v0.1.0-alpha.1` for server behavior and use Grafana-client source at that repository tag or in Git history for the old coupled implementation.
