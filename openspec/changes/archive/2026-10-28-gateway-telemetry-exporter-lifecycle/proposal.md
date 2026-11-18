# Proposal

## Why

The contracts and discovery cache in #590 and #591 do not own delivery clients. Customer recording needs isolated, versioned resources that survive active recordings during permitted rotation and stop exporting after observed revocation.

## What Changes

- Add a bounded internal exporter pool with typed factory output, per-stack/signal bindings, shared construction, and idempotent recording leases.
- Separate transport configuration from recording-policy revisions. Replace resources for changed destinations or credential versions without mutating existing client authentication.
- Supply a revocable, deadline-bound export gate and HTTP transport wrapper. Reject wrong destinations and redirects before authenticated transport use.
- Retire idle/replaced clients, suppress repeated construction failure, and bound shutdown while retaining accounting for non-cooperative work.
- Test the lifecycle with deterministic fakes and the pinned real OTLP HTTP exporter against local TLS receivers.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-customer-telemetry`: Client ownership, recording leases, transport revocation, rotation, and bounded retirement.

## Impact

Internal customer telemetry package and maintained Gateway architecture/glossary. No production route wiring, Cloud credential provider, global telemetry registration, new protocol, or SDK dependency upgrade. This PR extends existing stack #592 after #591.

The current AgentO11y generation HTTP client cannot inject this gate and drains queues on shutdown. Its direct transport adapter remains an explicit integration gate; no unsafe cancellation or queue-discard claim is made for that SDK path. The reusable pool is proved against a supported OTLP injection seam without changing the chosen generation-ingestion protocol.
