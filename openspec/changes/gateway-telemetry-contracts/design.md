# Design

## Context

See [proposal.md](proposal.md) for motivation and [the delta spec](specs/gateway-customer-telemetry/spec.md) for requirements. The inspected base is `3c3730d335cc4537395b60d6134062572a4a0e85`.

`internal/auth/access.go` already separates authorized namespace attribution from account access. `internal/service/model_observability.go` constructs one operator Agent Observability client; `internal/config/settings.go` owns its settings and resolves its secret separately. Neither supplies customer destinations. Reusing that client as a fallback would route customer observations to the wrong owner.

The registered upstream reference is Vercel AI `7.0.143`, provider `4.0.29`, commit `b7f6255acaae7724adc0fd37aac3dae3f3466578`. Its [model middleware wrapper](https://github.com/vercel/ai/blob/b7f6255acaae7724adc0fd37aac3dae3f3466578/packages/ai/src/middleware/wrap-language-model.ts) supplies execution composition, not hosted-stack discovery or ingestion authorization. This is a Gateway-specific extension, consistent with the coverage map's Agent Observability extension classification. No upstream pin or provider wire behavior changes.

## Goals / Non-Goals

**Goals:** Define a small internal contract with executable validation and policy decisions. Make independently changing destinations, eligibility, preferences, and credentials representable without conflating them. Enable subsequent cache and lifecycle development with deterministic fakes.

**Non-Goals:** Live discovery, transport construction, exporter pooling, customer-resource storage, authentication header formats, runtime activation, provider adapters, and customer logs. Do not introduce a setting that appears to enable delivery before the implementation exists.

## Decisions

### 1. Use a focused Gateway-internal package

Add `ai-gateway/cmd/grafana-ai-gateway/internal/customertelemetry`. Keep Go types and narrow resolver seams internal rather than creating a public telemetry framework. It must not depend on provider execution or import Agent Observability merely to describe destinations. Use the existing authorized caller boundary; a typed stack key is attribution, not an independent authorization mechanism.

The resolver input carries the authorized namespace as an opaque key. Cloud-specific namespace-to-stack mapping belongs in the later verified adapter. Reject empty/refused attribution before resolution and reject mismatched result ownership. Avoid parsing assumed numeric identifiers into the common contract.

### 2. Separate three snapshots

| Snapshot | Contents | Excludes |
| --- | --- | --- |
| Destination | Stack key, signal, endpoint, opaque ingestion identity, credential reference, revision, validity | Secrets, consent, customer-selected endpoints |
| Effective policy | Stack key, per-signal permission, fixed capture/metric profile, revision, validity | Endpoints, secrets, product activation |
| Credential reference/version | Opaque deployment-owned authority reference and rotation version | Caller/provider keys, assumed Basic/Bearer formatting |

Use typed signal constants for generations, traces, metrics, and logs. Validate HTTPS endpoints without URL user information, query, or fragment; destination allowlisting and redirect policy are adapter/transport responsibilities in later PRs. Support independent unavailable signals without inventing a fallback identity. Return fixed validation errors that do not include rejected values.

Keep credential material outside destination and policy values. This PR establishes references and independent revision semantics; concrete secret acquisition and authentication construction wait for verified ingestion requirements. A generic arbitrary-header map would obscure those requirements and is deliberately excluded.

Constructors defensively copy collections; getters must not expose mutable backing maps. Use explicit time inputs for expiry checks. Zero or expired validity cannot authorize recording. Keep policy revision independent from destination and credential changes so later policy-only updates need not rebuild transport clients.

### 3. Centralize a conservative default policy

Combine authoritative per-signal eligibility with mandatory service constraints and the optional preference state. Represent preference absence, explicit disablement, and lookup failure separately. The first runtime source will supply absence because there is no customer resource yet; test fakes can exercise disabled/failure states now. Do not simulate persistence or pretend those fakes are a supported customer configuration API.

Only an authoritative eligible signal with genuine preference absence receives defaults. Unknown eligibility and failed reads yield no new recording authorization. Cache reuse, refresh ordering, and disablement invalidation are owned by PR 3; this package supplies the distinguishable outcomes and expiry checks it needs.

The initial profile permits metadata-only generations and correlated model spans. It excludes content, raw errors, arbitrary request attributes, and provider/backend details. Customer logs are disabled even when a log destination exists. Model metrics are limited to operation duration, token usage, and streaming time to first token when available. The current Agent Observability dependency (`v0.19.0`) names these `gen_ai.client.operation.duration`, `gen_ai.client.token.usage`, and `gen_ai.client.time_to_first_token`; tool-call metrics and arbitrary extra families are excluded from this initial profile.

The initial metric dimensions are operation name plus closed error classification for duration, operation name plus token type for token usage, and operation name for first-token timing. Model and customer identifiers are excluded. The named profile version is part of policy comparison independently of the source revision.

This contract declares the permitted profile; it does not claim the SDK exports only those fields by default. The recording integration PR must explicitly constrain instrument families and dimensions and verify exported payloads, including errors before a result exists. New dimensions or capture modes require review, not an implicit SDK/default upgrade.

### 4. Preserve existing observation boundaries

No production composition root imports this package to activate export in PR 1. Operator telemetry settings and the independently usable application middleware retain their current semantics. No global tracer/meter provider is replaced.

Existing `gateway-plugin-telemetry` says the Gateway must not configure its own trace exporter; `gateway-text-observability` specifies one process-wide Agent Observability client. Those describe the existing operator path. Before adding customer exporters, the integration change must explicitly scope those requirements to operator telemetry and add the customer delivery requirements. This contract-only slice does not weaken or reinterpret the running implementation.

### 5. Keep acceptance local and meaningful

Table-driven tests cover two customers with deliberately different per-signal identities, malformed destinations, partial availability, unknown eligibility, absent versus disabled versus failed preferences, expiry, and mutation of constructor inputs. Assert credential reference/version independence without real secret material. Existing Gateway tests provide regression coverage; no live Cloud test is needed to claim these contracts work.

Later cache and pool PRs can consume fake resolvers while the real Cloud adapter is pending. They must remain disconnected from production delivery until verified adapters and authorization exist. The immediate-error conversation correlation work is needed for recording integration, not this package.

## Risks / Trade-offs

- **Internal abstractions precede a live adapter** → Keep the surface small and unexported outside the Gateway module; defer transport auth shapes and concrete control-plane fields.
- **Metadata-only is mistaken for automatic privacy enforcement** → This PR authorizes a bounded profile only. Later payload tests must establish actual generation, span, metric, and diagnostic filtering.
- **A destination is mistaken for product consent** → Eligibility and policy are separate prerequisites; unknown state cannot authorize recording.
- **Contract tests are mistaken for Cloud delivery proof** → Document the evidence boundary and keep runtime delivery inactive.

## Migration Plan

PR 1 adds internal code and tests without runtime wiring, new environment settings, or persisted customer state. Rollback removes unused contracts. Later default-policy consumers use the same boundary when customer preferences are introduced; no customer resource migration is required for stacks that have never set preferences.

Next independently reviewable slices are the verified discovery adapter (PR 2), request-time cache (PR 3), exporter/credential lifecycle (PR 4), recording integration (PR 5), full local Gateway proof (PR 6), and deployment/test harness (PR 7). PRs 3 and 4 can start from these contracts without waiting for PR 2's live service evidence.

## Open Questions

The following gate later integrations, not implementation of this contract:

- Which documented control-plane fields authoritatively identify each signal's destination, ingestion identity, and eligibility?
- Which deployment-managed write authority is accepted by each intended ingestion service, and how is its rotation exposed?
- What is the supported generation transport and customer trace context behavior for the deployed service?
- Which supported App Platform resource and update mechanism should supply later customer preferences?

No proposed answer to these questions is encoded as a production default here.
