# Design

## Context

See proposal.md for motivation, and `specs/gateway-request-routing` for the contract. Code on `main` after #555, #579 (#420) and #581:

- `execution.applyPolicy` already strips call-level `gateway` and hands its raw JSON to the selector. `service.NewSelector` branches on access:
  - configured access calls `execution.CatalogSelector`, which refuses any controls;
  - BYOK access calls `byok.DecodeRequest`, which accepts only `byok`.
- `service.buildCatalog` constructs every physical candidate once. Each is native, then wrapped in `nativeoptions` validation and `execution.Translate`. Multi-candidate routes are composed with `fallback.New(...).WithDecider(execution.FallbackDecider)` and a private physical-attempt observer, then wrapped once by the logical `ModelFactory`. Only the composed model and its `[]catalog.ConfiguredCandidate` descriptors are kept; the physical candidates are not reachable after startup.
- `byok.New` builds one provider/model chain from the selected provider's accounts. Anthropic gets a request-level `nativeoptions` check outside the chain, so a refused request makes zero attempts.
- `execution.Selection` carries one of two attributions:
  - `configuredSelection{candidates}`;
  - `RequestAccounts{Provider, ModelID, Count}` from #420.
- The attempt capture projects the flat `Overview` from it. `execution.Metadata` places `gateway.execution` and relocates native `gateway` to `gateway.nativeMetadata`. Every ProviderWire carrier (unary success, error documents, stream finish, committed stream errors) already calls it.
- The pinned `@ai-sdk/gateway@4.0.116` client posts `providerOptions` unchanged (`gateway-language-model.ts`; registered tests pass `order` and `providerTimeouts` through). The pinned repo's `00-ai-gateway.mdx` documents the `models`/`order`/`only` semantics. A recorded `harness-fx` fixture shows the service's `gateway.routing` shape. vercel.com was unreachable from the sandbox, so the current service docs were not re-read.

## Goals / Non-Goals

**Goals:**
- One composition path for both access modes, built from already-constructed physical candidates (configured) or request accounts (BYOK).
- Zero behavior change, and no per-request construction, for calls without controls.
- Every routing failure known from the request is decided before provider I/O.

**Non-Goals:**
- No change to `fallback`: its decider, commitment, drain and observer stay as they are.
- No change to the pinned TypeScript client, no client-side routing or validation in the Go client, and no new public catalog API.
- No `test/integration` scenario. UI chunk shapes don't change, and `providerMetadata` pass-through is already covered there.

## Decisions

### D1. One control decoder, exact top-level keys
A service-local decoder reads call-level `gateway` into `map[string]json.RawMessage`, so control names are matched exactly (Go struct decoding would fold case). It checks the keys against the caller's access mode, then decodes each value:
- `models` is decoded as `[]json.RawMessage`. A non-string entry is either an object (the conditional form) or malformed, and both fail with routing-controls.
- `order` and `only` are decoded as `[]string`.
- `providerTimeouts` is decoded as a map whose only member is `byok`.
- `byok` keeps `byok.DecodeRequest`'s existing validation.

Validation order follows the spec's diagnostic precedence. For BYOK, the existing credential and selector errors come after routing-controls and before fallback-model checks.
- *Alternative: extend `byok.DecodeRequest` for both modes.* Rejected. Configured callers must not go through credential decoding.

### D2. Physical route table beside the catalog
`buildCatalog` also returns a service-local table keyed by canonical ID. Each entry holds the route's ordered physical candidates (the wrapped instances the prebuilt chain uses), their `config.Primary` descriptors and their `catalog.ConfiguredCandidate` identities. The selector resolves every requested ID and `models` entry through the catalog resolver, which handles aliases, and then looks up the physical candidates in this table.
- *Alternative: expose candidates on `catalog.ResolvedModel`.* Rejected. It changes a public package to serve one host.
- *Alternative: construct candidates per request.* Rejected. That would mean per-request construction and repeated secret handling, and the spec requires startup-constructed accounts.

### D3. Pure composition, then one request chain
A pure function receives the ordered model groups plus `only` and `order` and returns the composed candidates with their group indexes. For each group it filters by `only`, then does a stable move-to-front by `order`. It skips repeated groups and repeated (provider instance, backend model) tuples, and returns no-candidates or attempt-bound errors before construction. The selector then builds `fallback.New(composed...)`, adds `FallbackDecider` and the physical observer over the composed descriptors, and wraps the result with the configured `ModelFactory` under the primary's canonical ID.

The pure function is table-tested. It applies to both modes: BYOK groups have one provider each, so `order` is a no-op there by construction.
- *Alternative: nest the prebuilt route models inside an outer fallback.* Rejected. It would wrap logical middleware per route and nest fallbacks, which breaks attribution (#420 D2) and cannot filter inside a route.
- *Alternative: truncate at the bound.* Rejected. It would hide which candidates were dropped.

### D4. Selection attribution becomes ordered model groups
`configuredSelection` and `RequestAccounts` are replaced by an ordered list of model groups, each holding its requested ID and expected destinations:
- configured groups also hold the canonical ID and `ConfiguredCandidate` identities;
- BYOK groups hold the native provider, model and account count.

`CatalogSelector` produces one group. `Project` and `projectAccounts` check decisions position by position against the flattened groups, keeping the fail-closed checks from #420 D2. The overview stays flat and unchanged on the wire. For routed configured calls, `canonicalModelId` is the primary's canonical ID.
- *Alternative: a separate routing attribution next to the existing ones.* Rejected. That would mean two sources of attempt identity for the same capture.

### D5. `gateway.routing` is projected from the overview
`attemptCapture.view` projects `Routing` from the validated overview and the groups, and `View` carries it. `execution.Metadata` gains the routing argument, so every carrier picks it up with no adapter changes. Projection is omitted, while the overview is kept, when the attempts can't be assigned to groups in order.

Field names follow the subset of the fixture's shape defined in the spec. `success` means "selected", matching the overview's semantics. Timestamps, request IDs, costs, credential type and `planningReasoning` are excluded, consistent with the overview contract and #420's "no credential-source field". `gateway_execution.json` gains an optional `routing` definition, and the strict TypeScript schema test covers it.
- *Alternative: emit routing only when controls are present.* Rejected. Vercel emits it on every call, and consumers would otherwise need two code paths.
- *Alternative: replace `gateway.execution`.* Rejected. #322 and #420 consumers depend on it.

### D6. BYOK multi-model composition
`byok.DecodeRequest` returns ordered models (`provider`, `model`, accounts) for the selector and its `models` entries. The full map validation is unchanged. `byok.New` takes the composed list and builds one flat fallback chain: one candidate per account, in model order, then account order.

The request-level Anthropic `nativeoptions` check stays outside the chain only when every composed candidate is Anthropic, which preserves today's zero-attempt refusal. Mixed chains rely on the per-candidate check that #581 already added, as configured mixed chains do.
- *Alternative: always pre-check Anthropic options.* Rejected. It would refuse a request because of a later candidate the call might never reach.

### D7. Provider start timeout via a first-response-byte trace
Each BYOK account candidate for a listed provider is wrapped, inside fallback and outside translation, with a start-timeout model. Per attempt, the wrapper:
- derives a cancel-cause context carrying an `httptrace.ClientTrace`;
- starts a timer, which `GotFirstResponseByte` stops exactly once;
- when the timer fires first, cancels the attempt context with a timeout cause.

The attempt then returns a single-chain error whose message is fixed and which unwraps to `context.DeadlineExceeded`. As a result:
- the failure summary carries the message;
- `ClassifyProvider` reports timeout;
- `DefaultDecider` treats the error as eligible;
- `fallback.decision` still sees the parent context live, so request cancellation stays authoritative.

The same "start responding" point (the first response byte) applies to unary and streaming calls, and it is observable through the shared transport, which forwards the request context. The attempt context is released when the parent ends, so no relay goroutine is needed.
- *Alternative: bound the whole unary attempt.* Rejected. It would cut off long but healthy generations.
- *Alternative: time to the first stream part.* Rejected. It needs a peek, has no unary equivalent, and the upstream docs say "start responding".

### D8. Closed categories and fixed documents
`execution` adds an invalid-routing category with reasons (routing controls, provider timeouts, no candidates, attempt bound) and a fallback-model reason on model-not-found. ProviderWire maps these to the fixed documents in the `providerwire-v4-unary-runtime` delta, and the unsupported-control message changes. Configured callers that send unsupported controls now get `ErrUnsupportedGatewayControl`. `ErrReservedProviderOptions` is kept for `CatalogSelector` and policy encoding failures.

### D9. Go client `GatewayOptions`
`providers/grafana` exports `GatewayOptions` with `ProviderKey() == "gateway"`, following the typed-option pattern of `anthropic.AnthropicOptions`. It is encoded by the client's existing `projectOptions` path through `json.Marshal`, with no new codec. The fields are:
- `Models`, `Order`, `Only []string`, all `omitzero`, so a nil slice is absent while an explicit empty `Only` is kept;
- `BYOK map[string][]BYOKAccount`, where an account has `APIKey`, `BaseURL`, `Organization` and `Project` (`apiKey`, `baseURL`, ... with `omitempty`, except the key);
- `ProviderTimeouts *ProviderTimeouts{BYOK map[string]int}`.

Provider keys stay plain strings, because the Gateway owns the supported set. The module must not import `ai-gateway`, so the types are client-owned.
- *Alternative: mirror all upstream fields.* Rejected. The Gateway always refuses those fields (decision 1a).
- *Alternative: validate before sending.* Rejected. The Gateway is the only authority (decision 3a), and upstream's language model doesn't validate either.

### D10. Go client metadata readers
`GatewayMetadataFrom(provider.ProviderMetadata) (*GatewayMetadata, error)` and `GatewayMetadataFromError(error) (*GatewayMetadata, error)` return nil and nil on absence.
- `GatewayMetadata` holds `Execution *ExecutionOverview` and `Routing *Routing`.
- The overview, attempt and failure types mirror the server JSON. `Outcome` is a typed `AttemptOutcome` string with `failed`, `selected` and `canceled` constants, and the failure `Code` stays `json.RawMessage`.
- `Routing` mirrors the spec's field subset.

The error reader uses `errors.As` to find the `*provider.APICallError`. It decodes `Data` and reads top-level `providerMetadata` (unary and setup documents) or `error.data.providerMetadata` (stream error frames), using the document shapes the client already retains. Decoding is ordinary `encoding/json` on a copy, and the metadata decoding contract is unchanged.
- *Alternative: hook decoding into the client's result mapping.* Rejected. Metadata stays opaque under `gateway-provider-metadata`, and a reader works equally on results the caller stored.
- *Alternative: separate execution and routing readers.* Rejected. They are one Gateway projection and are published together.

### D11. Evidence plan
- **Go:**
  - pure composition table tests;
  - control decoder cases, covering precedence and exact keys;
  - projection and routing unit and race tests;
  - service selector tests with fake native HTTP for Anthropic, OpenAI and compatible candidates, asserting the native request bodies and attempt order;
  - timeout tests with a fake server that delays headers, plus a context-cancellation race;
  - physical-record correlation tests.
- **Both registered clients:** `gateway-command.test.ts` drives the real command, in configured (self-hosted) and cloud BYOK modes, through the pinned client and the Go client. It covers configured-only, request-only and combined routing, unary and streaming, diagnostics and routing metadata.
  - Cross-population cases prove that no other credentials or endpoints are used.
  - Fakes with per-key failures come from #420.
  - A consumer-middleware case checks that metadata reaches Go middleware independently of server capture.
  - Go client cases use `GatewayOptions` and the metadata readers. Differential captures compare them with the pinned client's equivalent `GatewayProviderOptions` bodies (`go-client-differential.test.ts`).
- **Go client unit tests:**
  - option encoding: nil versus empty `Only`, omitted account fields, equivalence with raw options;
  - readers: absence, malformed members, numeric codes, unary, finish and stream-error carriers;
  - public-surface and module-isolation tests;
  - a logger test showing typed `apiKey` redaction.
- **Provenance:** all provider responses are synthetic. No `test/conformance` inputs change, and `knownGaps` is unchanged; the capability spec records the support boundary. The parity classification is a Gateway extension behind pinned-client pass-through.
- **Checks:** `mise run test-ai-gateway-command`, `mise run test-providerwire-v4`, plugin validation, `mise run parity-check` and the module checks.

## Risks / Trade-offs

- [The logical observation chain is now built per routed request] → It is cheap middleware wrapping, as BYOK already does per request. Unrouted calls keep the prebuilt model.
- [Every published overview now carries `gateway.routing`, which changes existing response bytes] → The change is additive within a Gateway-owned namespace that both clients treat as opaque. Exact-metadata tests are updated deliberately.
- [Message and category changes for unsupported controls] → Marked BREAKING in the proposal. Status, type and code stay invalid-request.
- [A native SDK issues a request without the attempt context, so the trace never fires] → The timeout then fires and the attempt fails safely. Fake-server tests for both adapters catch it.
- [The client's metadata types drift from the server JSON] → The command evidence decodes real Gateway responses with the readers, and `gateway_execution.json` remains the shared reference.
- [Slug mismatch with Vercel (for example `vertex` or `bedrock`)] → These slugs have no configured candidates here, so they simply don't match. The docs list the slugs that exist.
- [Skipping repeated tuples can hide a route an operator expected to retry] → The spec documents it, and it matches the startup rule against duplicate tuples within a route.

## Migration Plan

This needs no migration. Requests without controls are unchanged. Callers that relied on the old unsupported-control message or the reserved-provider-options refusal get the new documents. To roll back, revert the change: controls are then refused again, as before.
