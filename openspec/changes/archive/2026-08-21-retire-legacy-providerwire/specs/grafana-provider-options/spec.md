## REMOVED Requirements

### Requirement: GrafanaOptions typed provider option
**Reason**: `GrafanaOptions` is owned by the deleted legacy Grafana client module.
**Migration**: Consumers may use its source at repository tag `v0.1.0-alpha.1` or in Git history; no independently fetchable nested-module version is claimed.

### Requirement: Per-middleware hard-disable knob
**Reason**: The control types are removed with their owning client module.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history until strict host-control capabilities are introduced.

### Requirement: Graded Agent Observability capture control
**Reason**: The control type is removed with its owning client module.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history until strict host-control capabilities are introduced.

### Requirement: Attach via ProviderOptions
**Reason**: The concrete option attached through this mechanism no longer exists in the repository.
**Migration**: Generic provider options remain supported; use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history for `GrafanaOptions`.

### Requirement: Lossless wire round-trip
**Reason**: The round trip is specific to the retired transport and deleted concrete option type.
**Migration**: Pin root `v0.1.0-alpha.1` for server behavior and use Grafana-client source at that repository tag or in Git history for the historical behavior.

### Requirement: Client-side validation of known fields
**Reason**: The validating concrete option type is removed.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history for this validation.

### Requirement: Options carry intent for the server-side middleware stack
**Reason**: The deleted legacy client no longer publishes this host-control contract.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history; future strict host controls will be specified with the capability that implements them.
