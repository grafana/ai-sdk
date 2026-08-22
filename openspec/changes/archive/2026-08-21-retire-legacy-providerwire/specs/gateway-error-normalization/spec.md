## REMOVED Requirements

### Requirement: Normalized gateway error with type discriminator
**Reason**: `GatewayError` and its typed categories are exported only by the deleted legacy Grafana client module.
**Migration**: Consumers may use their source at repository tag `v0.1.0-alpha.1` or in Git history; no independently fetchable nested-module version is claimed.

### Requirement: GatewayError preserves the originating APICallError as cause
**Reason**: The wrapper type is removed with its owning module.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history; generic `provider.APICallError` remains available to retained providers.

### Requirement: Normalizer maps structured provider error to a normalized type
**Reason**: The normalizer belongs to the deleted legacy client and will not define the future strict protocol's safe error categories.
**Migration**: Use the Grafana-client source at repository tag `v0.1.0-alpha.1` or in Git history; strict safe-error mapping will be specified independently.
