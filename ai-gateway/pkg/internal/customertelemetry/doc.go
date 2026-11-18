// Package customertelemetry defines the Gateway's customer telemetry contracts,
// bounded discovery cache and exporter lifecycle. It has no live discovery or
// secret-service adapter, production exporter wiring, or recording activation.
// Production request handling does not yet consume this package.
//
// Stack attribution comes from an authorized auth.Caller. Destination discovery,
// product eligibility and recording preferences, and ingestion credentials have
// separate owners. A destination's existence never authorizes recording.
// CredentialRef contains a deployment-owned reference and rotation version, not
// caller authentication, provider keys, or an ingestion secret. Concrete secret
// acquisition and transport authentication belong to later delivery adapters.
//
// Snapshots are immutable after construction. Callers must not mutate input maps
// during construction. Resolve results must be checked for the requesting stack
// and current validity before use. Cache performs these checks on resolver output
// and refreshes destination and policy snapshots independently. Callers must still
// use RecordingDestination at selection time. Lookup outcomes are diagnostics,
// not recording authority. Cloud-specific identity mapping, egress restrictions,
// and secret rotation are separate integrations.
//
// Cache owns shared lookup cancellation; canceling one request only ends that
// request's wait. Cold requests reserve LookupTimeout plus InferenceHeadroom from
// their remaining deadline. Requests without a deadline still have a bounded
// metadata wait. Operational bounds must be supplied explicitly by the owner.
//
// A transient refresh failure preserves a snapshot only until its original
// expiry. Observed denial or invalid data clears it; valid replacements apply
// disablement and removal immediately. Previously returned snapshots cannot be
// revoked in place. Future recorder leases must enforce their own lifecycle.
// The owner must Close the cache with a finite shutdown context. Resolvers must
// honor cancellation and bound transport calls; a non-cooperative call remains
// charged to capacity until it actually returns, even after lookup timeout.
//
// Pool owns typed exporter resources through immutable bindings and recording
// leases. An ordered configuration owner calls Update; arbitrary request snapshots
// must not reconcile out of order. Acquire shares construction without retaining
// caller context values. Matching transport settings reuse a client across policy
// revisions; endpoint, signal identity, or credential-version changes replace it.
// Old leases may drain only while their original authority remains valid.
//
// Factories must install ExportGate at their real transport seam, use the exact
// credential reference/version, and bound their SDK queues/retries. The gate checks
// validity on every export and cancels in-flight operations after revocation. It
// cannot retract bytes already sent. HTTPClient refuses other targets and redirects.
// Shutdown errors quarantine capacity while the pool is open because teardown is
// unproven. The pool adds no queue or retry layer around an exporter.
//
// Local tests exercise supported HTTP client injection in the pinned OTLP exporter.
// The pinned AgentO11y generation HTTP client has no injectable transport and drains
// queues on Shutdown. Its direct adapter requires a supported gating seam before
// integration; this package does not claim to revoke that SDK's internal queue.
package customertelemetry
