# Tasks

## 1. Runtime: capture and selection facts

- [ ] 1.1 Keep observed attempts on the first seal and return none on later seals; add a non-sealing `observed()` and share one projection between `view` and the new read; verify `TestAttemptCapture_View` (view-once and late attempts unchanged) and a new non-sealing-read table (before entry, single candidate, fallback, after `View` and `Close`, no attribution).
- [ ] 1.2 Set the requested ID and capture on the selection in `selectModel` before composing, and add `RequestedID`, `CanonicalID`, `Expected` and `Attempts(err)`; verify runtime tests for a composer that reads them (configured alias, BYOK, before entry, after a stream's first-part `View`) and a selector's selection that reports none.

## 2. Public contract

- [ ] 2.1 Replace `Call.Model` with `RequestedModel` and `CanonicalModel`; add `Call.CallerIdentity`, `Call.Candidates`, `Candidate`, `AttemptOutcome`, `Attempt` and `Record.Attempts` with godoc; verify `go vet` and the external-module build test.

## 3. Extension host

- [ ] 3.1 Derive requested model, canonical model and candidates in `Host.Compose`, and give each extension's `Call` its own candidate copy; verify `TestPipeline_RequestInfo` with a mutating extension.
- [ ] 3.2 Read the selection's attempts when building each record (unary inner error; stream setup error; stream end: nil after a part, context error, or a fixed premature-end failure) and give each observer its own copy; verify pipeline tests over a real runtime for single candidate, fallback with 503, admit and inspect-request denial, output denial, a single-candidate stream closing before and after its first part, and a mutating observer, under `-race`.
- [ ] 3.3 Prove the capture cannot be replaced: an extension that derives a context with its own attempt observer gets no attempt, and the overview and records are unchanged; verify with a pipeline test over a real runtime.

## 4. Service: request info and attribution

- [ ] 4.1 Add `CallerIdentity` to `ExtensionRequest`; verify `TestExtensionRequest` over `Decide` for a user, a service account, an internal service alone and on behalf of a user, and `self-hosted`.
- [ ] 4.2 Add `callerIdentity` to the observation view in `ObserveAccess`, emit `caller_identity` in `observationLogAttrs` and `gateway.caller_identity` in `observationMetadata`, and accept both in the log allowlist and `allowedAgentMetadataKeys`; verify the updated `TestRequestObservation_AuthenticatedTrustBoundary`, the bound and absence tables, and `model_log_policy_test.go` (bounded string accepted; oversized and non-string refused).

## 5. Acceptance across sources

- [ ] 5.1 Extend `TestExtensionPipeline_EverySelectionGetsThePipelineOnce` (operator catalog, alias, routed, BYOK; unary and stream) with requested and canonical models, candidates, caller identity and record attempts; verify `go test ./pkg/internal/service/...`.
- [ ] 5.2 Add a stream fallback test whose consumer calls `View` at the first part while the fake provider holds the stream open; verify the stream-end record carries both attempts.
- [ ] 5.3 Show that an operator and a default-enabled tenant extension both see the UID; verify with a service test.
- [ ] 5.4 Show that model logs carry `caller_identity` and generations `gateway.caller_identity` for configured and BYOK calls, and neither for an internal service without a user; verify with observer tests.
- [ ] 5.5 Show that no metric series gains a label: after calls with a UID, label names per family are unchanged and no label value contains the UID; verify with a registry gather test.
- [ ] 5.6 Show that provider requests are unchanged: the observation bridge test asserts provider headers and options carry no UID, and a BYOK outbound test asserts the captured request bytes don't contain it; no request snapshot is regenerated, and `mise run test-providerwire-v4` passes against the unchanged semantic request goldens.

## 6. Docs

- [ ] 6.1 Update `docs/guides/gateway-extensions.md` (request information fields, record attempts, UID visibility for operator and tenant extensions) and the `extension` package doc; verify `mise run lint-docs` and that the example still matches the new `Call`.
- [ ] 6.2 Update `ai-gateway/docs/text-observability.md`: `caller_identity` in the log table and in Agent Observability metadata, and replace "user or service account IDs" in what the Gateway does not record with "metric labels never include them".
- [ ] 6.3 Check the glossary and architecture walkthrough against the result (caller identity, request information, record attempts) and update what drifted; verify `mise run lint-docs`.

## 7. Verification

- [ ] 7.1 `mise run fmt`, `mise run vet`, `mise run lint` and `mise run test`, with `GOFLAGS=-buildvcs=false` locally; `-race` for `extensionhost`, `execution` and `service`.
- [ ] 7.2 `mise run parity-check`, with no expectation changes; `openspec validate add-extension-request-info --strict`.
- [ ] 7.3 CI e2e and module-policy checks on the PR.
