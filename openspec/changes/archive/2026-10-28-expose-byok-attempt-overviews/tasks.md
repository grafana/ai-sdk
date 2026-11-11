# Tasks

## 1. Execution projection for request accounts

- [x] 1.1 Add request-account attribution to `execution.Selection`: native provider, native model and account count, with an exported constructor and godoc (design D1). Verify that `go build ./...` in `ai-gateway` passes and that a unit test shows the constructor's selection passes `validSelection`.
- [x] 1.2 Project request-account captures (design D2/D3):
  - requested selector, no canonical identity, decision identity, no instance;
  - omit the overview on a provider/model mismatch, more decisions than accounts, or zero attempts;
  - no single-candidate synthesis.

  Replace `TestAttemptCapture_BYOKCapturedNotProjected` with a table covering direct, failed→selected, noneligible, exhausted, canceled, mismatched provider, mismatched model, account overflow and zero attempts. Verify with `go test ./internal/execution/...`.
- [x] 1.3 Keep configured projection unchanged. Assert that existing configured capture/overview tests still pass without edits, and that a configured capture with an empty canonical identity still omits the overview. Verify with `go test ./internal/execution/...`.
- [x] 1.4 Make `Overview.CanonicalModelID` `omitempty`. Drop `canonicalModelId` from `required` in `providerwire/v4/schema/gateway_execution.json`, keeping `minLength: 1`. Add `schema.test.ts` cases that accept a missing canonical ID and reject an empty one, and update `schema-definitions.test.ts` if it pins `required`. Verify with `go test ./internal/execution/...` (JSON omits the key for request accounts) and `pnpm run test:schema` in `ai-gateway/test/providerwire-v4`.
- [x] 1.5 Add a race test with concurrent BYOK calls on one runtime with different account outcomes and keys. Assert that each view holds only its own attempts and summaries, and that attempts observed after sealing or after context end are dropped. Verify with `go test -race ./internal/execution/...`.
- [x] 1.6 Add BYOK scenarios to `TestLineAdapter_MatchesProviderWire` (failed→selected, exhausted) and assert the same attempt view, including the omitted canonical identity. Verify with `go test -run TestLineAdapter_MatchesProviderWire ./internal/execution/...`.

## 2. BYOK selector and ProviderWire carriers

- [x] 2.1 Make `service.NewBYOKSelector` return the request-account selection from `request.Provider`, `request.Model` and `len(request.Accounts)`. Verify with a selector unit test asserting the attribution and zero catalog calls.
- [x] 2.2 Rework `TestBYOKSelection_NativeSummariesAndAttributionGap` (renamed to drop "Gap") to assert:
  - unary and setup error bodies carry `providerMetadata.gateway.execution` with the redacted or echoed summary, exactly one native call and a matching observer count;
  - committed errors carry the overview plus the unchanged `nativeError`;
  - no `canonicalModelId`, no `providerInstance`, and unchanged fixed error fields.

  Verify with `go test ./cmd/grafana-ai-gateway/internal/service/...`.
- [x] 2.3 Add a service test that pins adapter identity: Anthropic and OpenAI BYOK attempts report provider `anthropic`/`openai` and the unmodified native suffix, including a suffix with extra slashes. Add a test that an Anthropic consumed-option rejection publishes no overview and makes zero native calls. Verify with `go test ./cmd/grafana-ai-gateway/internal/service/...`.
- [x] 2.4 Add ProviderWire runtime tests for a BYOK-attributed selection on every carrier: unary success, unary error, setup error, stream finish and committed error. Include a native `gateway` namespace relocated under `nativeMetadata`, and extend the existing configured-key/BYOK-echo redaction test to cover the overview summary. Verify with `go test ./providerwire/v4/...`.

## 3. Both-client command integration

- [x] 3.1 Extend `FakeAnthropic` and `FakeOpenAI` in `gateway-command.test.ts` to accept a set of dummy keys with per-key failure statuses, plus an OpenAI HTTP-200 initial SSE error mode. Record each request's key so per-key counts and order can be asserted. Verify that existing command tests still pass with `pnpm run test:command`.
- [x] 3.2 Add a Cloud BYOK matrix test (Anthropic and OpenAI × unary and stream × direct success, direct failure, 503→selected, 401 stop, 503/503 exhausted) through the pinned Vercel client and the Go client capture. Assert:
  - identical ordered attempts, outcomes and summaries in result/finish/error metadata for both clients;
  - native per-key counts and order matching the attempts;
  - unchanged status, type and retryability;
  - no `canonicalModelId`, `providerInstance` or unused-account key in output.

  Verify with `mise run test-ai-gateway-command`.
- [x] 3.3 Add an OpenAI preflight case: the first account returns an HTTP-200 initial SSE error and advancement follows the default decider. Both clients must see the failed attempt's summary. Verify with `mise run test-ai-gateway-command`.
- [x] 3.4 Update the existing BYOK command tests that assert absence: the echo/configured-key test's `!includes('"execution"')` and the 401 test's exact `apiError.data`. They should now assert the overview and its summary (BYOK echo preserved, configured key redacted) while fixed classification is unchanged. Verify with `mise run test-ai-gateway-command`.
- [x] 3.5 Add a command test with concurrent BYOK calls using distinct keys and outcomes, plus a canceled stream during setup. Assert there is no cross-request attempt or summary data, that no extra native request starts after cancellation, and that no overview is invented. Verify with `mise run test-ai-gateway-command`.

## 4. Documentation

- [x] 4.1 Update "Inspect execution overviews and failures" in `docs/providers/grafana-gateway.md`:
  - BYOK calls return overviews: requested `provider/model`, native attempts in account order, no `canonicalModelId` or `providerInstance`, earlier failed accounts with summaries;
  - remove the "don't return overviews yet (#317)" note;
  - keep the sensitivity and BYOK-echo guidance.

  Add a one-line pointer from the BYOK section. Verify with `mise run lint-docs`.
- [x] 4.2 Update the glossary "Diagnostics; execution overview" row and the "What the observations tell you" paragraph in `docs/concepts/gateway-architecture.md` to cover configured and BYOK attempts. Check the BYOK flow diagram for drift. Verify with `mise run lint-docs`.
- [x] 4.3 Update the godoc for `execution.Selection`, `Overview` (optional canonical identity) and the package overview where it describes attempt capture. Verify with `go doc ./internal/execution` in `ai-gateway`.

## 5. Integration checks

- [x] 5.1 Run `mise run test-ai-gateway`, `mise run test-race`, `mise run vet` and `mise run lint`, and confirm they pass.
- [x] 5.2 Run `mise run test-providerwire-v4` and `mise run test-ai-gateway-source-integration`, and confirm they pass.
- [x] 5.3 Run `mise run validate-parity-baseline` and `mise run parity-check`, and confirm that no conformance inputs or expectations changed (`git status test/conformance` is clean).
- [x] 5.4 Run `openspec validate expose-byok-attempt-overviews --strict` and confirm it passes.

## Workflow follow-up

- Open a draft PR titled `feat(ai-gateway): expose BYOK attempt overviews and precommit failure summaries` referencing #420. State that all evidence is synthetic, not live-provider evidence.
- Archive the change after review.
