## REMOVED Requirements

### Requirement: Public complete provider-wire package and dependency boundary
**Reason**: The public tolerant server package is retired and will not coexist with the future strict protocol adapter.
**Migration**: Hosts requiring the legacy handler must pin `github.com/grafana/ai-sdk@v0.1.0-alpha.1`.

### Requirement: Request-aware model resolver API
**Reason**: The resolver API is exported only by the removed legacy handler.
**Migration**: Pin the root rollback version; future strict resolver composition will be specified independently.

### Requirement: Handler configuration and defaults
**Reason**: These options and defaults configure only the retired handler.
**Migration**: Pin the root rollback version until strict construction limits are available.

### Requirement: Provider-wire request validation
**Reason**: The requirement defines tolerant legacy validation rather than the planned strict dialect.
**Migration**: Pin the root rollback version for legacy validation behavior.

### Requirement: Bounded canonical CallOptions decoding
**Reason**: Direct decoding into provider-domain `CallOptions` is removed as HTTP protocol authority.
**Migration**: Pin the root rollback version; future strict decoding will use a schema and explicit mapper.

### Requirement: Unary model dispatch and response
**Reason**: The unary execution path belongs to the removed legacy handler.
**Migration**: Pin the root rollback version for this handler behavior.

### Requirement: Streaming model dispatch and response commitment
**Reason**: The streaming execution path belongs to the removed legacy handler.
**Migration**: Pin the root rollback version for this handler behavior.

### Requirement: Canonical SSE framing, flushing, and termination
**Reason**: This framing contract is owned by the retired transport and is not the future strict SSE authority.
**Migration**: Pin the root rollback version for legacy SSE.

### Requirement: Error normalization and commit-aware encoding
**Reason**: The requirement serializes provider-domain errors through the retired public transport.
**Migration**: Pin the root rollback version; the strict runtime will define safe errors independently.

### Requirement: Request cancellation propagation
**Reason**: This lifecycle contract applies only to the removed handler.
**Migration**: Pin the root rollback version until strict lifecycle behavior is available.

### Requirement: Total timeout behavior
**Reason**: The timeout sentinel and behavior belong to the removed handler.
**Migration**: Pin the root rollback version until strict total-duration limits are available.

### Requirement: Streaming idle timeout behavior
**Reason**: The timeout sentinel and behavior belong to the removed handler.
**Migration**: Pin the root rollback version until strict idle-duration limits are available.

### Requirement: Observable stream output failure handling
**Reason**: This writer-failure behavior applies only to the removed legacy SSE implementation.
**Migration**: Pin the root rollback version until the strict stream lifecycle is available.

### Requirement: Real Grafana client/server conformance without module cycles
**Reason**: Both packages and their joint conformance path are retired.
**Migration**: Pin root `v0.1.0-alpha.1` for the historical server and use the Grafana-client source at that repository tag or in Git history; no nested-module version is claimed.

### Requirement: Provider transport parity and Assistant migration boundary
**Reason**: The repository no longer claims parity for the retired transport; strict pinned-client evidence is deferred to the next work package.
**Migration**: Keep deployed legacy servers on root `v0.1.0-alpha.1`; Grafana-client source remains available at that repository tag or in Git history until strict migration guidance exists.
