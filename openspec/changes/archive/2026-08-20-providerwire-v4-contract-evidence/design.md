## Context

Phase 1 starts from `c0e19b316edda4cfde8bb08f0398950a576d68b9`, equal to the reviewed `origin/main` parent. The repository currently has a tolerant legacy `gateway/providerwire` codec and handler whose request codec delegates to `encoding/json` on `provider.CallOptions`. That transport remains deployed regression context; it is not the strict V4 authority.

The sole compatibility baseline is `test/conformance/upstream.yaml`: Vercel AI SDK commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`, `ai@7.0.65`, `@ai-sdk/gateway@4.0.52`, `@ai-sdk/provider@4.0.7`, and the other registered exact packages. Phase 1 must establish source equivalence before repository source supports a claim, then derive the request contract from the exact provider request declarations, referenced unions, Gateway serializer and header composition path, and derive non-2xx probe expectations from the exact Gateway failed-response and error-normalization path installed from those packages.

Current provider types use pointers for several optional scalars but use non-pointer booleans and plain strings elsewhere. Nil versus non-nil empty collections and required values selected by a discriminator remain semantically representable even when generic `encoding/json` tags omit them; the future V4 codec owns their explicit mapping. Flat tagged structs may also admit invalid inactive arms without losing valid distinctions. Phase 1 measures semantic provider-domain representability; Phase 2 owns any provider API redesign.

## Goals / Non-Goals

**Goals:**

- Define the strict ProviderWire V4 request envelope and JSON payload contract for the exact registered Gateway client.
- Make every finite request member and discriminator reviewable through one authoritative classification.
- Capture deterministic semantic requests for distinct serializer, schema, presence, exclusion, or sequencing behavior.
- Demonstrate every current Go representation loss with an executable, passing Phase 1 witness and a Phase 2 delta entry.
- Add truthful smoke evidence for unary JSON, SSE clean EOF, and non-2xx client consumption.
- Make ordinary verification non-mutating and artifact updates explicit and reproducible.

**Non-Goals:**

- Changing `provider.CallOptions`, prompt/tool types, provider implementations, or orchestration.
- Implementing `gateway/providerwire/v4`, a production decoder, handler, Go client, policy layer, authentication, or service.
- Changing or tightening the tolerant legacy ProviderWire dialect.
- Defining exhaustive response arms, server error categories, privacy policy, lifecycle behavior, or malformed-request runtime handling.
- Claiming compatibility with Vercel's private server, arbitrary Gateway versions, or real provider responses.

## Decisions

### 1. Separate the protocol contract from its evidence system

`providerwire-v4-request-contract` owns the normative HTTP request and JSON Schema contract. `providerwire-v4-contract-evidence` owns classification, captures, probes, and maintenance. The existing `provider-wire` and `gateway-providerwire-server` capabilities remain unchanged.

This avoids conflating a future strict dialect with the deployed tolerant transport and keeps test tooling from becoming the protocol definition. A single combined capability was rejected because it would make maintenance mechanics normative protocol behavior.

### 2. Use an explicit authority order

The authority order is:

1. exact versions in `test/conformance/upstream.yaml`;
2. source-equivalent installed npm package declarations and runtime code;
3. raw requests emitted through the installed Gateway serializer by deterministic scenarios.

Provider request declarations and referenced unions define the surface that must be classified, while the installed serializer, header composition path, and emitted requests determine the request wire contract. A declared item that the serializer cannot emit is recorded as an explicit exclusion rather than overriding observed HTTP behavior. The black-box non-2xx probe derives its expected behavior from the installed Gateway failed-response and error-normalization path, which is included in source-equivalence evidence. The repository checkout at the registered commit may explain behavior only after its relevant sources are shown equivalent to the installed packages. Current Go types, legacy codecs, generated schemas, and documentation are comparison targets, not authorities.

Source equivalence uses a manually maintained, grouped closure of the provider request/response declarations, Gateway language-model serializer and configuration path, provider-utils request/header/response helpers, and Gateway error-normalization path. Exact package pins remain the broad package guard. Hashing unrelated image, video, or other package sources was rejected as upgrade noise, while an AST or automatic import graph was rejected as unnecessary machinery.

### 3. Keep one typed source-derived coverage map

One canonical TypeScript coverage map records every relevant call-options key, finite object key, prompt/tool/file discriminator, Gateway transform, and exclusion. Each entry carries its upstream identity through the category and member, a named scenario and semantic JSON pointer with either exact expected data or a presence requirement, or an explicit exclusion rationale.

Typed helpers and `satisfies` constraints cover complete finite keys and discriminated unions, so pinned additions and removals fail typechecking. The generic evidence validator consumes the same map and verifies every observation. `classification.json` is generated from this source as a reviewer-facing artifact with derived baseline metadata; it is never maintained independently. Exact empty, zero, false, discriminator, and transformed values live in the map. Only relational header composition, JSON Schema `$ref`/`$defs` preservation, and multi-step sequencing retain focused assertions outside the generic observer. This provides compile-time and behavioral drift detection without an AST or custom source-diff framework.

### 4. Capture raw requests transiently and commit stable semantic evidence

The local HTTP recorder retains each raw request until it has:

1. classified the method, the `/language-model` path relative to the configured `baseURL`, content type, protocol headers, and behavior-affecting outer headers as either a supported strict envelope or an explicit collision exclusion;
2. parsed the body with a strict, duplicate-member-aware JSON path;
3. asserted the expected runtime observations, including body/outer duplication, exact-key replacement, and pre-Fetch lower-case last-value normalization outcomes;
4. normalized only insignificant JSON object ordering and explicitly allowed authentication, user-agent, and observability header values.

Committed captures preserve request sequence, method, relative path, final Fetch-emitted content type, protocol and classified behavior-affecting outer headers, envelope classification, and semantic JSON. Array order, absence, null, empty collections, zero, false, tagged-union selection, non-exempt header values, and collision outcomes remain significant. Raw bytes need not be committed because byte formatting is not the compatibility contract; semantic captures must not be cited as evidence of malformed-syntax or duplicate-member rejection.

Header composition has three relevant stages. Gateway configuration headers first pass through `withUserAgentSuffix` and `normalizeHeaders`, so their names are already lower-case. The serializer then uses case-sensitive exact-key object spread in this order: configured Gateway headers, call-level `options.headers`, model protocol headers, and observability headers; `postJsonToApi` prepends the default JSON content type. Only a later identical property key replaces an earlier one at this intermediate stage. Before Fetch, `postToApi` calls `withUserAgentSuffix` again; `normalizeHeaders` iterates the intermediate entries in insertion order, lowercases every key, and lets each later case variant replace the earlier normalized value. Fetch therefore receives one value per normalized name rather than combined case variants. Call-level headers are asserted both in the serialized body and on the outer HTTP request. Authentication, user-agent, and observability values may be normalized as explicitly client-owned, but intermediate exact-key and final normalized last-value outcomes remain observable.

Reserved collisions are evidence. Exact and case-variant protocol-header inputs resolve to the later model-derived value and can remain supported strict requests. Exact or case-variant `Content-Type` call headers occur after the prepended default during normalization and can replace it with a non-JSON value; those cases map to explicit exclusions and fail strict envelope validation. Supported captures must retain one clean value for content type and each `ai-language-model-*` header.

A maintained JSON parser/visitor with explicit rejection of duplicate members, comments, trailing commas, invalid numbers, and trailing data will be selected during implementation. A hand-written general JSON parser is rejected. Focused parser tests remain independent from JSON Schema validation.

### 5. JSON Schema draft 2020-12 is the payload authority

Hand-authored, reviewable draft 2020-12 schemas under `test/providerwire-v4` define the request payload and shared unions. Schemas close objects where the registered contract is finite, distinguish required from optional properties, encode nullability explicitly, and use discriminated `oneOf` branches that reject inactive-arm properties. Opaque provider option values remain semantic JSON and are not recursively constrained beyond their container contract.

A maintained validator such as Ajv may execute the schemas, but generated TypeScript or validator output is not normative. Captures and targeted negative cases independently check the schemas, preventing one generator from serving as both producer and oracle.

### 6. Group scenarios by distinct behavior

Scenarios combine compatible fields into readable behavior groups rather than producing one fixture per member. The minimum behavioral families are:

- direct unary scalar and presence behavior;
- direct streaming request behavior;
- system, user, assistant, and tool prompt roles;
- every file-data and request-content union arm;
- function tools, provider tools, tool choice, tool results, and approvals;
- structured output and opaque provider options, including nested nulls;
- call-level headers in both the body and outer request, including intermediate exact-key configured/call/protocol/observability composition, pre-Fetch lower-case last-value normalization, and unsupported content-type collisions;
- explicit empty collections;
- multi-step client tool flow and request ordering.

A new capture is justified only by a distinct serializer, schema, presence, sequencing, or exclusion behavior. Model IDs and generated values are deterministic.

### 7. Model semantic Go losses as passing external-package witnesses

The Phase 2 delta table records the request item, valid pinned request distinction, evidence scenario/path, current provider-domain behavior, executable witness, and required model change. A row qualifies only when an explicit V4 mapper cannot recover the distinction from transport-neutral Go values. Generic `encoding/json` tags and output are not protocol authority: nil versus non-nil empty collections and required members selected by discriminators remain representable and are codec responsibilities rather than provider-model losses.

Each witness runs from `package provider_test` and passes by asserting that the public model loses or cannot construct the distinction. Phase 2 will first invert the relevant witness into a failing regression test, then implement the redesign. Byte-format differences, invalid inputs, permissive inactive arms, and `omitempty`-only output differences do not qualify.

### 8. Keep response evidence deliberately smoke-level

Locally authored deterministic probes invoke the pinned Gateway client as a black box against a minimal local server and cover only:

- unary JSON consumption;
- one complete JSON stream part per SSE `data:` event with clean EOF and no required `[DONE]`;
- safe non-2xx parsing whose expectation is derived from the source-equivalent installed Gateway failed-response and error-normalization path together with the represented status.

The error-path sources are included in the equivalence evidence, while the assertion executes only through the public installed client. These probes establish client consumption, not valid server output for every response arm, a local server taxonomy, or production lifecycle behavior. Exhaustive response schemas and runtime conformance belong to later phases.

### 9. Extend existing baseline, aggregate, and upgrade tooling

`test/providerwire-v4` is added to the pnpm workspace and to the explicit consumer lists used by `validate-baseline.mts` and `upgrade-baseline.mjs`. Semantic requests, generated classification, source equivalence, and workspace pins all derive from or validate against `upstream.yaml` and the workspace manifest; stale metadata fails focused non-mutating tests. The complete ProviderWire check runs from aggregate parity, repository check, and required CI rather than relying on package-version validation alone. Upgrade tooling updates exact pins and makes regenerated coverage, capture, schema, and probe differences review-visible with the rest of the baseline change.

This extends the existing parity governance rather than creating a second manifest or dependency resolver.

### 10. Expose exactly two maintenance workflows

- `mise run check-providerwire-v4` installs test dependencies as needed, typechecks, verifies exact baseline pins and the committed source-equivalence attestation against installed package inputs, regenerates evidence in memory or a temporary directory, compares it with committed artifacts, validates schema/envelope/observations, runs Go loss witnesses, and runs response probes. It does not repeat the upstream source-tree comparison and must leave the worktree unchanged.
- `mise run update-providerwire-v4-artifacts` explicitly rewrites generated semantic captures and reviewer-facing classification, then invokes the complete check. A parity upgrade separately refreshes the explicit relevant source-equivalence closure and reviews the canonical typed coverage map, maintained schemas, loss decisions, and probes when affected.

Git is the rollback mechanism. No transaction framework or hidden write mode is added.

## Risks / Trade-offs

- **[Pinned declarations and runtime serializer differ]** → Inventory both, classify every serializer transform, and require emitted-request observations.
- **[Semantic normalization erases contract distinctions]** → Normalize only object ordering and approved volatile headers; preserve presence, null, empty values, numbers, booleans, arrays, and request order.
- **[Schema and implementation share the same mistake]** → Hand-author reviewable schemas, validate pinned-client captures, and add independent targeted negative cases.
- **[Type exhaustiveness creates unreadable machinery]** → Limit native TypeScript maps to finite exported keys and discriminators; do not build an AST framework.
- **[Response probes overstate compatibility]** → Label inputs as local client-consumption probes and document untested response/runtime surfaces in the parity map.
- **[The Go delta contains unrelated redesigns]** → Apply the coherence gate before handoff; if losses do not form one provider-contract change, revise subsequent phases rather than broadening Phase 2 silently.
- **[Normal checks rewrite evidence]** → Generate into temporary storage during checks and assert a clean diff after the workflow.
- **[A package upgrade silently changes the contract]** → Include the workspace in exact baseline validation and coordinated upgrades, with compile-time and semantic drift failures.

## Migration Plan

This phase adds evidence and contracts only. Introduce the workspace and baseline registration first, then classification, captures, schemas, loss witnesses, response probes, and workflows. Preserve the legacy transport and provider API throughout. Rollback consists of reverting the new workspace, workflow, parity-map, and OpenSpec changes; no production migration or persisted data is involved.

The Phase 2 handoff is the reviewed request schema, complete classification, semantic corpus, and coherent delta table. No Phase 2 branch should begin if package/source equivalence fails or the delta coherence gate fails.

## Open Questions

- Which maintained duplicate-aware JSON parser best satisfies strict syntax requirements with the smallest dependency and test surface?
- Which exact request artifacts should be generated versus maintained manually once the source-derived classification format is prototyped?
- Does the complete inventory demonstrate one coherent Phase 2 provider-contract redesign? This is intentionally answered by Phase 1 evidence before handoff.
