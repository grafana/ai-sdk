## Purpose

Provide production self-hosted Gateway authentication without a Cloud account or JWT issuer while preserving configured-account authorization.

## ADDED Requirements

### Requirement: Explicit boot authentication configuration
The bounded strict provider/model YAML SHALL accept auth.type jwt, static-key or unsafe and optional boolean server.cloud.enabled defaulting true. Only omitted auth SHALL use legacy JWT/unsafe settings. Null/empty auth, null selected blocks, invalid scalar types, unknown/duplicate fields, YAML merge keys and aliases, additional documents, unselected blocks and explicit-auth conflicts with legacy JWKS or enabled unsafe SHALL fail with secret-safe errors.

#### Scenario: Omitted versus null auth
- **WHEN** auth is omitted or explicitly null
- **THEN** only omission SHALL preserve legacy selection, and null SHALL fail before secret lookup or binding

### Requirement: Selected authentication validation
JWT SHALL require jwksURL and retain audience ai-sdk and verifier behavior. Unsafe SHALL require development and loopback on all active listeners. Startup SHALL validate routes, active listeners and selected endpoints before secret resolution and binding; JWT-specific limits and timeout latency SHALL apply only to JWT.

#### Scenario: YAML JWT
- **WHEN** YAML selects JWT with valid trust configuration
- **THEN** existing Go and Vercel credentials SHALL authenticate with unchanged claims and configured-account policy

### Requirement: Static credential snapshots
Static authentication SHALL accept 1–64 named identities with shell-name environment references, resolve each distinct reference once in sorted identity order, reject missing/empty/duplicate credentials and retain only fixed digests. Exact keys SHALL be 1–4096 ASCII bytes in bearer syntax with optional trailing equals padding. Constructor inputs SHALL not allow later mutation. All credentials SHALL be compared without an early-match return. No credential refresh SHALL occur until restart.

#### Scenario: Restart replaces credentials
- **WHEN** the environment key changes
- **THEN** the running process SHALL retain its original key, and a restarted process SHALL accept the new key and reject the old one
- **AND** admitted requests SHALL retain existing shutdown semantics

### Requirement: Fail-closed local admission
Static authentication SHALL accept exactly one Authorization Bearer or X-Access-Token value, reject duplicate/case-colliding/comma-coalesced headers and invalid or oversized tokens, and reject X-Grafana-Id, X-Scope-OrgID, X-Cloud-Org-ID and X-Access-Policy-ID even when empty. It SHALL not read query/body credentials. Rejection SHALL return the fixed 401 before body/catalog/provider work.

#### Scenario: Ambiguous credentials
- **WHEN** a request supplies both credential headers or identity assertions
- **THEN** it SHALL fail without protected work or credential-bearing diagnostics

### Requirement: Local identity and policy
A matched identity SHALL have static-key source, configured-account policy, service name, local:name subject, empty namespace, zero stack and no acting user. Names SHALL provide attribution only; all entries have the same permissions. Static callers SHALL not obtain request-BYOK access; Cloud callers SHALL not obtain configured accounts.

#### Scenario: Both clients use local credentials
- **WHEN** Go NewWithAccessToken or pinned Vercel createGateway sends the valid opaque secret
- **THEN** configured discovery, unary and streaming SHALL succeed without JWT parsing or Cloud identifiers

### Requirement: Standalone production composition
With Cloud disabled, the process SHALL neither construct Cloud handlers nor bind its address, and SHALL ignore that inactive address in validation. All active listeners SHALL share rollback, readiness and shutdown. Production SHALL permit disabled Agent Observability only for static-key with Cloud disabled. Enabled export SHALL retain all validation; disabled export SHALL resolve no credential or start workers. The operational listener SHALL remain separate and unauthenticated.

#### Scenario: Production standalone bootstrap
- **WHEN** production selects static-key, disables Cloud and export, and supplies native provider configuration
- **THEN** exactly private and operational listeners SHALL start without JWKS construction or requests

#### Scenario: Other production combinations
- **WHEN** production uses JWT, unsafe or enabled Cloud with export disabled
- **THEN** startup SHALL fail

### Requirement: Credential-safe operations and evidence
Errors, logs, observations, metric labels and upstream headers SHALL exclude inbound credentials; source/outcome metrics SHALL remain bounded. Guides SHALL describe entropy recommendations, opaque JWT-looking values without expiry, equal permissions, restart rotation and mixed replicas, safe proxy replacement, TLS termination and private operational ingress.

#### Scenario: Credential privacy
- **WHEN** successful or rejected static requests carry sentinel secrets
- **THEN** diagnostics and upstream requests SHALL exclude the sentinels while permitted nonsecret caller attribution remains available

### Requirement: Production image evidence
Image evidence SHALL test production native-endpoint discovery and Cloud-port closure from the container network and distinguish synthetic tests, local skips and Linux acceptance.

#### Scenario: Linux source-image bootstrap
- **WHEN** the source-image test runs on Linux with Docker
- **THEN** it SHALL prove production static-key discovery and closed Cloud ingress in the built image
- **AND** other environments SHALL report an explicit skip without claiming Linux acceptance

### Requirement: Temporary hashing buffer cleanup
Boot and request hashing SHALL validate keys before copying into a bounded application-owned mutable buffer and SHALL explicitly clear that buffer after hashing. Environment provisioning and HTTP-owned strings SHALL remain unchanged. This SHALL NOT imply erasure of environment, compiler, runtime or hashing-internal copies.

#### Scenario: Owned temporary buffer cleanup
- **WHEN** a valid static key is hashed at boot or for admission
- **THEN** the owned temporary raw-key buffer SHALL be cleared after its digest is computed
- **AND** exact credential matching and restart snapshot behavior SHALL remain unchanged
