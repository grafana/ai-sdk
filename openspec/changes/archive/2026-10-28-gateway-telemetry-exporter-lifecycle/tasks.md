# Tasks

## 1. Resource ownership

- [x] 1.1 Implement validated options, ordered per-stack/signal bindings, typed factory resources, shared bounded construction, fixed errors, and idempotent leases; test invalid/disallowed state, cold/warm sharing, waiter cancellation and independent stacks.
- [x] 1.2 Implement configuration replacement without rebuilding for policy-only revisions; test rotation with active leases, obsolete bindings, same-target validity refresh and partial-build cleanup.

## 2. Export authority

- [x] 2.1 Implement expiry/revocation-aware export gates and exact-target HTTP wrapping with redirect refusal and finite request deadlines; test queued/in-flight revocation, target mismatch, expiry, and independently allowed signals.
- [x] 2.2 Exercise pinned OTLP HTTP export through actual local TLS receivers; verify protobuf requests, distinct stack/signal identities, old/new credential versions, SDK queue/retry behavior and shutdown after revocation. Document unsupported direct AgentO11y HTTP injection.

## 3. Lifecycle and bounds

- [x] 3.1 Bound current/retiring/building resources, suppress construction retries, reclaim idle resources and reject late construction; use deterministic tests including non-cooperative factory/cleanup behavior.
- [x] 3.2 Implement bounded idempotent close, drain leases before forced revocation, and exactly-once cleanup; test acquire/release/rotation/close races under the race detector.

## 4. Review and publish

- [x] 4.1 Update godoc, architecture, glossary and local review diagrams; verify no production wiring, SDK upgrade, secret values or private infrastructure details enter the diff.
- [x] 4.2 Run candidate Gateway tests, vet, pinned lint, formatting, baseline/docs checks and strict specs; review the final diff and prepare completed requirements for sync/archive.

## Publication follow-up

Sync and archive completed requirements before commit, sign the commit, create a draft against #591, and add it to existing GitHub stack #592. This publication sequence is not runtime implementation or evidence of Cloud delivery.
