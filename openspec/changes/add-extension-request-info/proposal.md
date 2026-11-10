# Proposal

## Why

Extensions only see the call's namespace, key ownership, source, caller type, correlation ID and one model ID. Quota, eligibility, billing and audit extensions also need to know who called, which model was requested and which canonical model it resolved to, which candidates the call may try, and which physical attempts it made. The caller's identity UID is excluded from every log and export today, so audit trails cannot name the user or service account behind a call.

## What Changes

- `extension.Call` gains the caller identity UID (`user:<uid>` or `service-account:<uid>`), the requested model ID as sent, the canonical model ID (empty for request BYOK) and the planned candidates in order (requested model or `models` entry, its canonical ID, provider, provider instance, provider model). They replace the single `Model` field, so the vocabulary matches the execution overview. **BREAKING** for the v0 extension API.
- `extension.Record` gains the call's physical attempts: provider, provider instance, provider model, outcome (selected, failed or canceled) and status code, the i-th attempt being the i-th planned candidate. They come from the runtime's own attempt capture, projected like the execution overview: a single candidate reports one attempt, a call refused before the model reports none, and a stream's attempts are available at stream end. No credential-bearing or provider-originated text is included.
- The runtime's attempt capture keeps observed attempts readable after it is sealed, so a stream's record still sees them after the adapter projected its overview at the first part. Composed per-call layers read the call's requested model, canonical model, expected attempts and projected attempts from the selection.
- The capture stays unreplaceable: extensions never return contexts or wrap the model, so they cannot install their own fallback observer.
- The caller identity UID is emitted as `caller_identity` in Gateway logs and as `gateway.caller_identity` in Agent Observability generation metadata, for every source. It never becomes a metric label and is never sent to providers. Other user claims stay private.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-extensions`: request information, the observation record's attempts and the unreplaceable capture.
- `gateway-caller-access`: "Verified caller attribution" allows the identity UID in extensions, logs and Agent Observability exports, still excluded from metric labels and provider requests.
- `gateway-text-observability`: the trusted observation view carries the caller identity UID.
- `gateway-execution-runtime`: composed layers read the call's selection facts and attempts without sealing the capture.

## Impact

- **Code:** `ai-gateway/pkg/extension` (`Call`, `Candidate`, `Attempt`, `AttemptOutcome`, `Record`); `ai-gateway/pkg/internal/extensionhost` (request info, attempts in records); `ai-gateway/internal/execution` (selection accessors, non-sealing capture read); `ai-gateway/pkg/internal/service` (request info, observation view, log allowlist, Agent Observability metadata allowlist).
- **APIs:** breaking change to the v0 `extension.Call` (`Model` becomes `RequestedModel` and `CanonicalModel`); additive `Call.CallerIdentity`, `Call.Candidates` and `Record.Attempts`.
- **Docs:** `docs/guides/gateway-extensions.md`, `ai-gateway/docs/text-observability.md`, the Gateway glossary and architecture walkthrough.
- **Parity:** not parity-sensitive. Request info, attempts and attribution are Grafana-owned; provider requests and ProviderWire bytes are unchanged, and no request snapshot is regenerated.
