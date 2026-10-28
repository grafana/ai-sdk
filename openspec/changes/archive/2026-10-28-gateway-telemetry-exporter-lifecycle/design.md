# Design

## Context

#590 defines immutable stack, destination and policy contracts; #591 caches them. Neither constructs exporters. Existing operator sinks own shutdown under the host's finite budget; this change remains unconnected to that composition root.

The registered Vercel reference is AI 7.0.143/provider 4.0.29 at b7f6255acaae7724adc0fd37aac3dae3f3466578. Middleware wraps model invocations rather than managing hosted customer exporters. This is an intentional Gateway-specific extension with no provider or frontend wire change.

Inspected installed dependencies: AgentO11y Go v0.19.0 `GenerationExportConfig`, `newHTTPGenerationExporter`, `Flush`, and `Shutdown`; OpenTelemetry OTLP HTTP v1.47.0 `WithHTTPClient`, `WithEndpointURL`, and exporter tests. AgentO11y's HTTP transport is internal and Shutdown closes then drains its queues. OTel supports an injected client. Therefore no direct AgentO11y HTTP adapter is included or falsely claimed revocable. No SDK upgrade or experimental generation-to-span migration is part of this slice.

## Goals / Non-Goals

**Goals:** Reusable resource ownership, tenant/signal isolation, immutable credential generations, bounded construction and retirement, supported export gating, and real HTTP export evidence.

**Non-Goals:** Cloud discovery or secret-service implementation; product activation; payload production/filtering; new queues/retries; persisted policy; production defaults or host wiring. Exporter adapters own bounded SDK queue/retry configuration and supported credential acquisition.

## Decisions

### Ordered configuration and typed leases

Use a generic internal pool with a typed resource value and a shutdown callback. A factory receives the authorized stack, signal, immutable destination (including credential reference/version), and an export gate. It resolves credentials through its deployment-owned dependency and builds an isolated resource. The pool stores no raw credential and never passes request context values to construction.

An ordered configuration owner calls Update with current destination/policy snapshots for a stack and signal. Update returns a binding; Acquire on that binding shares construction and returns a lease. Consumers do not pass old snapshots into Acquire. Replaced bindings cannot acquire again, and cannot reinstall their old configuration. Update is an internal reconciliation operation, not an API for arbitrary concurrent request snapshots; the future cache integration must serialize publication per stack. Snapshot revisions are opaque and cannot be ordered lexically.

Resource identity is stack, signal, endpoint, ingestion identity, and credential reference/version. A policy revision or refreshed validity alone does not rebuild transport. Consumers use their own immutable policy snapshots for capture. An observed restriction revokes all current and retiring bindings for that stack/signal, including active leases. The current default profile only changes enabled state; future capture-profile expansion must revisit compatibility explicitly.

### Export gate

The gate checks hard validity at each export, supplies a finite operation deadline, and cancels in-flight operations on revocation. Its HTTP wrapper permits only the configured full endpoint without userinfo/query/fragment and refuses redirects through the provided client. It passes no request-origin authentication through an implicit fallback. Real factories must install the gate at their actual export seam; merely having a gate object is not sufficient.

Revocation prevents queued data from starting a new network request; in-flight requests receive cancellation, which cannot retract bytes already accepted by a remote server. A supported SDK may drain its queue during Shutdown, but gated sends are denied. Count/report such failures through bounded adapter diagnostics; raw SDK transport errors must not become customer/operator labels. Protocol-specific queue retention and SDK diagnostics require integration proof.

### Rotation and validity

A permitted destination/credential change installs a new binding atomically. Existing leases may finish through the old immutable target until their original validity expires; newly acquired leases use the replacement. Old resources shut down only after the last lease releases, except at forced shutdown deadline. Removed/denied/invalid metadata revokes instead of draining through previous authority. Validity refresh can extend a matching active binding only from a fresh authoritative Update; expired/retired bindings are never resurrected.

### Bounds and shutdown

Explicit options bound retained resources (including pending, retired and closing ones), construction timeout, failed-build retry delay, export timeout, idle retention, maintenance cadence, and shutdown timeout. No deployment defaults are invented. One maintenance loop drives cleanup. Slow factory/shutdown code remains charged until actual return; timed-out work cannot multiply replacement goroutines. Acquisition waits on shared construction with independent caller cancellation. Failed construction returns fixed sentinels and must clean up any partial resource supplied with an error.

Close stops acquisitions and configuration changes, cancels construction, and permits leases to finish until the earlier of the configured shutdown bound and the first Close caller's deadline. Then gates are revoked and remaining clients are shut down. Each resource shutdown callback is invoked once with a finite context. Repeated Close calls can wait for outstanding work but never restart shutdown. Real factories, transports and shutdown callbacks must honor cancellation.

A shutdown callback returning an error does not prove teardown. Revoke its gate and quarantine its capacity for the lifetime of the open pool; do not retry cleanup or use the freed slot for more resources. On pool closure no more admission is possible, so accounting can finish with a fixed cleanup-error result. Successful partial-build cleanup also revokes the old gate before a later construction gets a fresh gate.

## Risks / Trade-offs

- The generation HTTP SDK lacks the required gate seam. A supported upstream hook or verified alternative remains required before direct generation transport is integrated; no globals or SDK-internal access is used.
- An ordered Update owner is essential. Out-of-order raw snapshot publication is not supported; later cache wiring must provide this ordering and revocation notifications.
- Gate validity bounds delivery, including buffered records. Expired/revoked queued data is deliberately lost; there is no lossless or exactly-once promise.
- Capacity includes draining generations, so rapid rotation can omit telemetry until old clients retire. This is safer than unlimited retained clients.
- Local dummy-auth TLS receivers establish isolation and SDK compatibility, not Cloud CAP scopes, real usernames, or deployment rotation behavior.
