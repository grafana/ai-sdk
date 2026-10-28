# Tasks

## 1. Attribution and destination contracts

- [x] 1.1 Add the focused internal customer telemetry package and authorized-stack input boundary; verify empty/refused attribution and cross-stack results cannot produce a recording plan using credential-free unit tests.
- [x] 1.2 Add typed signal identities, independent destination availability, revisions, validity, and credential references; verify two stacks with different IDs for every signal never substitute or share identities, and unavailable generations do not invalidate an otherwise valid metric destination.
- [x] 1.3 Implement destination validation and defensive snapshot copying; verify malformed/credential-bearing URLs, missing ingestion identities, zero/expired validity, mutation of input collections, and fixed diagnostics with no rejected values.
- [x] 1.4 Document the new internal contract in package godoc and the Gateway architecture/glossary where affected; verify terminology distinguishes telemetry destination discovery from model discovery and makes clear that live customer delivery is not implemented by this slice.

## 2. Effective recording policy

- [x] 2.1 Implement independent signal eligibility and default policy evaluation with distinct absent, disabled, and failed preference states; verify eligible absence selects defaults, unknown/ineligible signals remain disabled, and a failed read cannot restore recording after disablement.
- [x] 2.2 Define the fixed metadata-only capture and model-metric profile, with customer logs disabled; verify no raw-content option, arbitrary attribute, tool-call metric family, or operator-log forwarding is enabled by default, and an available endpoint cannot confer eligibility.
- [x] 2.3 Add independent policy revision/validity and immutable snapshots; verify expiry, policy-only disablement, credential-version changes, and input mutation without network access or sleep-based tests.
- [x] 2.4 Document the default profile and extension boundary for later customer preferences in the maintained Gateway documentation; verify it does not claim customer resource persistence, live transport enforcement, or full agent-workflow visibility.

## 3. Credential ownership boundary

- [x] 3.1 Finalize the narrow resolver/reference seams consumed by later discovery and exporter work; exercise them with test fakes and verify destination/policy values never contain secret material or assume an ingestion authentication scheme.
- [x] 3.2 Verify reference and revision handling is independent of caller authentication, provider keys, and operator exporter configuration; add focused tests for that separation and corresponding godoc without adding a public SDK API or dependency.

## 4. Integration acceptance

- [x] 4.1 Run the new package's tests, including the race detector where shared snapshots are exercised, and the repository's candidate-source Gateway test target; confirm existing auth, configuration, and operator telemetry regression coverage passes.
- [x] 4.2 Run formatting, applicable lint/vet checks, the parity baseline validator, and strict validation of this OpenSpec change; record exact commands and results. Do not claim provider parity or Cloud delivery from these checks.
- [x] 4.3 Inspect the resulting diff and production composition root to confirm no customer exporters, discovery calls, feature-enable settings, protocol changes, or unrelated POC changes were introduced. Verify the public artifacts contain no private infrastructure references or credentials.

## Workflow follow-up

- Review this as the first independent PR for #588. Keep the POC in its existing worktree and keep internal reports untracked.
- Archive the change only after implementation and the repository's review requirements are satisfied.
- Start request-cache and exporter-lifecycle changes from the accepted contracts; keep live Cloud adapter mappings and write-authority validation as explicit later gates.
