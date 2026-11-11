# Design

## Context

See proposal.md for the motivation. Today's code:

- Every ProviderWire carrier already encodes whatever `execution.View` holds: unary success (`handler.go`), unary/setup error documents, stream finish and committed stream errors (`stream.go`). Redaction (`SummaryRedactor`) and namespace relocation (`execution.Metadata`) are also shared.
- `newAttemptCapture` copies requested/canonical identity and candidates only from `Selection.configured`. BYOK selections (`service.NewBYOKSelector` → `execution.Selection{ID, Model}`) leave both IDs empty, so `Project` returns nil even though the BYOK attempts were captured.
- `byok.New` always wraps accounts in `fallback.New`, even when there is one account. So each invoked account yields one `fallback.Attempt`, carrying the native model's `Provider()`/`ModelID()` (`anthropic`/`openai` plus the native suffix) and a candidate-local `SourceErr`. The OpenAI HTTP-200 preflight error is a setup error and arrives as `SourceErr`.
- Anthropic BYOK wraps the chain in `nativeoptions.Model`. A consumed-option rejection therefore enters the model but produces zero attempts.
- The upstream baseline (`@ai-sdk/gateway@4.0.116`) defines no response-side routing metadata. `gateway.execution` is owned by the Gateway and passed through opaquely by both registered clients.

## Goals / Non-Goals

**Goals:**
- Publish BYOK attempts through the existing capture → `Project` → `View` path, with no adapter or client decoder changes.
- Fail closed: if the request's own data cannot confirm that observed attempts match it, publish nothing.

**Non-Goals:**
- No BYOK marker or credential-source field, and no account index or key fingerprint.
- No new SSE event, no change to UI message chunks, and no cross-language `test/integration/` scenario. The UI chunk shape is unchanged, and `providerMetadata` pass-through is already covered there.
- No change to fallback, deciders, classification, logging or exporter policy. Routing (#316), full native diagnostics (#323), client decoder cleanup (#375) and resource-policy refactoring (#394) are out of scope.

## Decisions

### D1. Request-account attribution travels on `Selection`
Add a second unexported attribution to `Selection`, next to `configured`. It holds the request-account destination: native provider, native model and supplied account count. An exported execution constructor builds it, because the BYOK selector lives in the service package. `NewBYOKSelector` passes `request.Provider`, `request.Model` and `len(request.Accounts)`.

- *Alternative: publish for any selection without configured attribution.* Rejected. Arbitrary selectors and test models would gain overviews whose identities come from whatever wrappers report.
- *Alternative: synthesize N `catalog.ConfiguredCandidate` copies.* Rejected. It reuses a configured type for non-configured data, and `Project` would then overwrite actual decision identity with request data instead of checking it.

### D2. Projection uses actual decision identity, checked against the request
For request-account captures, the overview carries:
- `requestedModelId` = the `provider/model` selector;
- no canonical identity;
- per attempt, the decision's own `Provider`/`ModelID` with no instance.

All existing `Project` ordering and outcome checks still apply. In addition, the overview is omitted when:
- any decision's provider or model differs from the request destination, or
- there are more decisions than supplied accounts.

This guards against nested or mixed fallbacks and adapter identity drift. A unit test pins the current adapter identities (`anthropic`, `openai`, model unchanged), so drift shows up as a test failure rather than silently missing overviews.

Configured projection is unchanged. Canonical identity stays required for configured captures.

### D3. No synthesized attempt for request accounts
The existing single-candidate synthesis applies only to direct (non-fallback) configured models that produce no decisions. Request-account captures never synthesize. If no attempts were observed (selection, credential or consumed-option rejection), no overview is published, matching "zero credential attempts".

### D4. `canonicalModelId` becomes optional on the wire
`Overview.CanonicalModelID` gains `omitempty`. `gateway_execution.json` drops `canonicalModelId` from `required` and keeps `minLength: 1` for when it is present. This is the user-approved option. Absence means no catalog resolution happened, whereas copying the selector into it would imply a resolution that never occurred.

### D5. Delivery, redaction and lifetime are reused unchanged
- `View` already redacts each attempt summary with the configured-key policy, and that runs before any carrier encodes it.
- Capture sealing, `discard` on context end and per-call observer registration already prevent late callbacks and cross-request mixing. This change adds tests, not new mechanisms.
- BYOK results with a native `gateway` namespace will now have it relocated under `gateway.nativeMetadata`, as configured results already do.

### D6. Evidence plan
- **Go:**
  - execution unit and race tests for capture and projection;
  - the test-only line adapter compared with ProviderWire;
  - ProviderWire runtime tests per carrier;
  - service selector tests with fake native HTTP. The existing `TestBYOKSelection_NativeSummariesAndAttributionGap` flips from asserting absence to asserting the overview.
- **Both registered clients:** `gateway-command.test.ts` drives the real command in Cloud mode through the pinned Vercel client and the independent Go client. `FakeAnthropic`/`FakeOpenAI` gain per-key failure statuses so ordered account chains can be exercised, and per-key request logs give native attempt counts and order.
- **Provenance:** all provider responses are synthetic fakes. No `test/conformance` inputs are added or changed. Parity classification: Gateway-owned metadata, parity-preserving for clients that treat it opaquely.

## Risks / Trade-offs

- [Native adapter changes its `Provider()` string, such as `openai.responses`] → D2 omits the overview (fail closed), and the pinned identity unit test catches the change.
- [Consumers assumed `canonicalModelId` is always present] → Configured output is unchanged. Docs state it is absent for BYOK, and the schema test covers both shapes.
- [BYOK echoes now appear in more places: overview summaries, unary error bodies] → This is intended by #420 and the existing caller-owned-echo policy. Configured-key redaction still applies, and docs keep the "treat as sensitive" guidance.
- [Existing tests assert the absence of `"execution"` for BYOK] → These are updated deliberately as part of this change, not loosened.

## Migration Plan

This is an additive response field for BYOK callers and needs no migration. To roll back, revert the selector attribution: BYOK selections then fall back to today's "captured, not published" behavior.
