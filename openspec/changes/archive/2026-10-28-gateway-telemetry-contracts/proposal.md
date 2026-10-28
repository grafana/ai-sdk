# Proposal

## Why

The Gateway currently records operator telemetry, but cannot independently route model observations to an authenticated customer's stack. The first slice of [#588](https://github.com/grafana/ai-sdk/issues/588) establishes the internal contracts and conservative defaults needed to implement that delivery without coupling customer identity, recording consent, and ingestion credentials.

## What Changes

- Add Gateway-internal, signal-specific destination and credential-reference contracts, keyed by an authorized stack identity.
- Add an effective recording policy with independent eligibility, revision, and validity. Start with metadata-only generations and traces, a fixed model-metric profile, and no customer log recording.
- Distinguish a missing optional customer preference from a disabled policy, unknown eligibility, and a failed policy lookup. Only genuine absence can select defaults.
- Test validation, signal and stack isolation, immutable snapshots, and conservative policy decisions without network access.
- Document the extension boundary for subsequent discovery, caching, exporter lifecycle, and recording integration PRs. Do not activate customer export in this change.

## Capabilities

### New Capabilities

- `gateway-customer-telemetry`: Customer telemetry destination, policy, and credential separation. This first change covers internal resolution and validation behavior; subsequent changes add live delivery.

### Modified Capabilities

None. Existing operator telemetry, provider execution, and client protocol requirements remain unchanged.

## Impact

Implementation is limited to a new focused package under `ai-gateway/cmd/grafana-ai-gateway/internal/`, deterministic tests, and affected documentation. No public SDK API, new dependency, customer configuration resource, provider protocol change, or deployment requirement is introduced. Production startup does not construct customer exporters or perform discovery.

Real control-plane field mappings, product eligibility, transport authentication, credential rotation, and live ingestion remain evidence gates for later PRs. The defaults do not grant product access or ingestion authority. The existing conversation-correlation PR is not a dependency of this contract-only slice.
