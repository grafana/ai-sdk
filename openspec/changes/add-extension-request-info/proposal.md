# Proposal

## Why

Extensions only see the call's namespace, key ownership, source, caller type, correlation ID and one model ID. Quota, eligibility, billing and audit extensions also need to know who called, which model was requested and resolved, which candidates the call may try, and which physical attempts it made. The caller's identity UID is also excluded from every log and export today, so audit trails cannot name the user or service account behind a call.

## What Changes

- `extension.Call` gains the caller identity UID (`user:<uid>` or `service-account:<uid>`), the requested model ID as sent, the canonical model ID (empty for request BYOK) and the planned candidates in order (requested model or `models` entry, canonical ID, provider, provider instance, provider model). It replaces the single `Model` field, so the vocabulary matches the execution overview. **BREAKING** for the v0 extension API.
- `extension.Record` gains the call's physical attempts: provider, provider instance, provider model, outcome (selected, failed or canceled) and status code, the i-th attempt being the i-th planned candidate. They come from the runtime's own attempt capture, projected like the execution overview: a single candidate reports one attempt, a call refused before the model reports none, and a stream's attempts are available at stream end. No credential-bearing or provider-originated text is included.
- The capture stays unreplaceable: extensions never return contexts or wrap the model, so they cannot install their own fallback observer.
- The caller identity UID is emitted as `caller_identity` in the logical model logs and as `gateway.caller_identity` in Agent Observability generation metadata, for every source. It never becomes a metric label and is never sent to providers. Other user claims stay private.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-extensions`: request information and the observation record.
- `gateway-caller-access`: "Verified caller attribution" allows the identity UID in extensions, logs and Agent Observability exports, still excluded from metric labels and provider requests.
- `gateway-text-observability`: the trusted observation view carries the caller identity UID.

## Impact

- **Code:** `ai-gateway/pkg/extension` (`Call`, `Candidate`, `Attempt`, `Record`); `ai-gateway/pkg/internal/extensionhost` (request info and attempts); `ai-gateway/internal/execution` (expected attempts and observed attempts on the selection; the capture keeps sealed attempts readable); `ai-gateway/pkg/internal/service` (observation view, log allowlist, Agent Observability metadata allowlist, request info).
- **APIs:** breaking change to the v0 `extension.Call` (`Model` becomes `RequestedModel` and `CanonicalModel`); additive `Record.Attempts`.
- **Docs:** `docs/guides/gateway-extensions.md`, `ai-gateway/docs/text-observability.md`, the Gateway glossary.
- **Parity:** not parity-sensitive. Request info, attempts and attribution are Grafana-owned; provider requests and ProviderWire bytes are unchanged, and the request goldens must stay unchanged.
