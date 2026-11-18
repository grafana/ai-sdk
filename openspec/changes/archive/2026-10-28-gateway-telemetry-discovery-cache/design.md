# Design

## Context

See [proposal.md](proposal.md). This change builds on #590, head `9cabee9b`, whose `customertelemetry` package provides immutable destination and policy snapshots, authorized stack attribution, two resolver interfaces, and recording-destination selection. It has no cache or live consumer.

`gateway.sinks.close` already bounds operator shutdown using a shared context. The new cache exposes a compatible bounded close operation, but this change does not add it to that composition root. Future integration must register it with the host lifecycle.

The registered Vercel baseline remains AI 7.0.143/provider 4.0.29 at `b7f6255acaae7724adc0fd37aac3dae3f3466578`. The pinned model middleware wrapper covers invocation composition, not customer metadata caches. This remains a Gateway-specific extension; provider, SSE, and client behavior do not change.

## Goals / Non-Goals

**Goals:** Coordinate existing resolvers with bounded latency and memory; preserve independent destination and policy freshness; make cancellation, invalidation, and shutdown races deterministic and testable.

**Non-Goals:** A real Cloud adapter, export clients, model execution, product activation, customer policy persistence, process-wide telemetry registration, or production feature settings. No startup enumeration or static customer list.

## Decisions

### 1. Keep resolution separate from inference

Add a cache in the existing internal package, constructed with `DestinationResolver`, `PolicyResolver`, and validated operational limits. Its lookup accepts an authorized `Stack` and a request context, returning immutable snapshots plus bounded component outcomes. Callers still use `RecordingDestination` to check signal permission and validity at selection time. There is no model callback and therefore no path for replaying inference.

Construction performs no lookup. Production routes do not call the cache in this PR. The eventual integration must call it only after authorization for model invocations, not catalog requests. This slice can prove invalid/zero attribution causes no resolver invocation, but does not claim route integration already exists.

### 2. Two component states per stack

Each admitted stack owns independent destination and policy state: the last validated snapshot, its expiry, next refresh/retry time, last fixed outcome, and at most one outstanding lookup per component. Keep entry bookkeeping under a mutex; execute resolvers outside it. Cold component lookups can run concurrently, so two metadata reads do not consume two sequential wait budgets. No lock is held across network work or while waiting for another caller.

Check result ownership and validity again at completion, using the actual current time. A resolver may return a snapshot that was valid when it started but expired while it was blocked. Destination success replaces the whole destination snapshot, including removals; policy success replaces the policy snapshot, including all-denied state. Component refreshes never rebuild or mutate the other component.

### 3. Request waiting does not own shared work

An admitted lookup runs under the cache lifecycle context with a finite lookup timeout. Request context values, bearer tokens, and provider keys do not become shared resolver credentials. Waiters independently select completion, their own cancellation, the shared timeout, or cache closure.

Before starting or joining a cold wait, compare the request deadline with the configured lookup timeout plus inference headroom. If it cannot accommodate that budget, return a budget outcome. A request without a deadline still has the finite metadata timeout. A warm usable result returns immediately. The cache never changes the request's inference deadline.

Canceling one waiter leaves shared lookup work intact. The cache may finish and retain the bounded result even after all original waiters have left. Avoid deriving shared cancellation from the first request or retaining that request's entire context.

### 4. Refresh, failure, and revocation rules

Use these transitions independently for destinations and policy:

| Result | State transition |
| --- | --- |
| Valid positive snapshot | Replace snapshot; schedule refresh before its deadline. |
| Valid empty destination or non-recording policy | Replace previous state immediately; recheck on the negative refresh interval. |
| Transient failure or timeout | Retain a previous snapshot only until its original expiry; apply retry suppression. |
| Authorization denial or invalid/mismatched result | Clear that component immediately; suppress retries briefly. |
| Snapshot expires | Stop returning it as usable, even if refresh or retry is pending. |

Target refresh of active entries at the earlier of the positive refresh interval and half their remaining lifetime; negative results target the negative refresh interval. Enforce the configured retry interval as minimum spacing between attempts, including successful responses that repeat an almost-expired snapshot. A short lifetime can expire before the next permitted attempt; stop recording at expiry instead of creating a tight refresh loop. Transient retries use the same fixed cooldown. Maintenance processes due work without refreshing entries past idle retention.

Keep last successful disabled state distinct from lookup failure; a failed read never selects defaults. Add a narrow sentinel for authoritative resolver denial, which later adapters can return through `errors.Is`. Existing validation errors map to invalid-data outcomes; `ErrPolicyUnavailable` and unclassified resolver failures map to transient lookup failure. A valid policy with no allowed initial signals is classified as non-recording, without guessing whether the source reason was consent, product eligibility, or service limits. Never inspect raw error text.

This cache stops new selections after an observed restriction. It does not revoke a recorder lease already issued by a future exporter pool. Buffered/in-flight data rules belong to the pool/integration work, and must not be claimed here.

### 5. Explicit bounds and lifecycle

Require positive limits for maximum entries, lookup timeout, positive refresh interval, negative refresh/retry interval, idle retention, and maintenance cadence. Inference headroom is nonnegative. Require retry spacing no greater than the positive refresh interval, and validate cadence against refresh/retention intervals. Add no production defaults or environment variables in this slice. Unit-test values are fixtures, not rollout recommendations.

A single maintenance worker services admitted entries, waking at the earlier of the next due action and the maintenance cadence. At most two resolver calls per entry can be outstanding. On capacity pressure, prune idle entries without active calls; otherwise return a capacity outcome. Background refresh does not count as customer activity or extend idle retention. Keep bookkeeping for canceled calls until the resolver actually returns, so repeated timeouts cannot spawn an unbounded number of abandoned goroutines.

Each attempt has a unique object identity and completion state. Timeout or close invalidates publication from that attempt. A late return cannot populate a removed entry or replace newer state. A non-cooperative resolver keeps its bounded slot occupied; do not replace it indefinitely. Production resolver implementations must honor context cancellation and enforce transport timeouts. Go cannot forcibly stop arbitrary resolver code.

Close is idempotent: stop admissions, cancel the lifecycle context, stop maintenance/timers, wake waiters, and wait for owned work only within the supplied close context. Late completions can release bookkeeping but cannot publish snapshots. No close operation invokes exporter flush because this package owns no exporters.

### 6. Deterministic verification

Use channel-controlled fakes, an explicit time source, and Go's `testing/synctest` where timers/context deadlines must advance together. Tests assert call counts and state transitions, not wall-clock scheduling guesses. No new dependency is required.

Cover two stacks, simultaneous cold requests, a canceled waiter, different component expiries, deadline headroom, transient refresh while valid and after expiry, observed disablement followed by failure, signal removal, retry suppression and later enablement, capacity reclamation, and close/completion races. Include a resolver that temporarily ignores cancellation, then release it during cleanup: prove callers return, its late result is ignored, and retries do not create replacement goroutines while it remains outstanding.

## Risks / Trade-offs

- **Background refresh adds load** → Only admitted active entries refresh; bound cardinality, concurrency, and retry cadence. Measure actual control-plane traffic before rollout.
- **Transient failures preserve old permission briefly** → Never exceed original validity; observed denial clears immediately. Document the refresh/expiry envelope with deployment values before activation.
- **Generic errors cannot reveal authorization semantics** → Add a typed denial sentinel and require later adapters to map authoritative responses. Do not infer permission from strings or unavailable fields.
- **Non-cooperative resolver cannot be killed** → Bound outstanding slots, release callers/close on deadline, reject late results, and require cancellation-aware real adapters.
- **Fake evidence is mistaken for live discovery** → Keep production wiring absent and document precisely what the tests establish.

## Migration Plan

The cache is an unused internal addition in this PR. No customer data, configuration, or runtime migration occurs. Review it as a stack on #590 and retarget to `main` after that PR merges. The verified discovery adapter can proceed separately; a later recording-integration PR constructs the cache with measured limits and registers shutdown under the existing host lifecycle.

## Open Questions

Before live integration, measure lookup latency and metadata lifetimes to choose deployment limits and the eligibility-revocation envelope. Determine the real resolver's error mapping from documented service responses. Neither question blocks the cache state machine or its deterministic tests; this PR supplies no guessed Cloud mapping or production timing guarantee.
