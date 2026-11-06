// Package customertelemetry defines the Gateway's customer telemetry contracts.
// It does not discover stacks, resolve secrets, construct exporters, or activate
// recording. Production request handling does not yet consume this package.
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
// and current validity before use. Cloud-specific identity mapping, egress
// restrictions, cache refresh, and secret rotation are separate integrations.
package customertelemetry
