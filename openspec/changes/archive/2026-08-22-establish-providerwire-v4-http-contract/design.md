## Context

Work package 1 removed the tolerant `gateway/providerwire` server and `providers/grafana` client. The next runtime work packages need a stable contract for the different protocol emitted by `@ai-sdk/gateway`, but no strict server or Go client exists yet.

The registered authority is `test/conformance/upstream.yaml`: `@ai-sdk/gateway@4.0.52`, `@ai-sdk/provider@4.0.7`, `@ai-sdk/provider-utils@5.0.27`, and upstream commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`. At that baseline, `GatewayLanguageModel` removes `abortSignal`, base64-encodes supported inline file bytes, posts JSON to `{baseURL}/language-model`, composes configured/call/protocol/observability headers in that order, parses unary and stream bodies with permissive schemas, filters unrequested raw stream parts, converts response-metadata timestamp strings to `Date`, and tolerates `[DONE]` through the provider-utils event parser. Header composition first uses case-sensitive JavaScript object keys and only later undergoes case-insensitive Fetch header normalization, so a call-level case variant can become the final emitted value even when the protocol layer later assigns the canonical lowercase key.

The TypeScript client is authoritative for what it emits and what it can consume. It is not complete server-response authority: unary handling spreads the server body and then replaces `request`, `response`, and `warnings` with client-owned values, and both success parsers accept arbitrary JSON. Later Go runtimes therefore need explicit response DTOs and schemas independently of these probes.

This change spans the initial top-level `ai-gateway/` module and license boundary, a production schema namespace, a private TypeScript workspace, the shared pnpm workspace and lockfile, baseline validation, task registration, and parity documentation. It creates contract evidence only; it does not add an executable HTTP handler. The root SDK remains independently Apache-2.0 and buildable without the Gateway module.

## Goals / Non-Goals

**Goals:**

- Establish `ai-gateway/` as an isolated AGPL-3.0-only Go module with one-way dependency enforcement, clear contribution scope, and provenance notices while preserving the root Apache-2.0 license.
- Define the complete JSON request projection of the registered `LanguageModelV4CallOptions` contract.
- Make finite upstream request and response surface drift fail at TypeScript compile time.
- Record compact, reviewable HTTP request semantics emitted by the real registered `createGateway` client.
- Prove focused unary, SSE, and non-2xx consumption behavior through the registered client.
- Make normal verification deterministic and non-mutating, with golden updates as a separate explicit action.
- Register the new workspace as a parity consumer and make its checks part of the repository parity signal.
- Preserve a clear evidence boundary between client compatibility and future strict server behavior.

**Non-Goals:**

- Revoke or alter licenses already granted for published revisions, claim legal confirmation, or invent a corresponding-source mechanism before Grafana legal approves it.
- Implement HTTP envelope validation, raw JSON processing, schema application in Go, provider-domain mapping, catalog resolution, model invocation, errors, unary encoding, or streaming state machines.
- Define unary, stream, or error response schemas for the future server.
- Generate the production request schema or a runtime feature-classification artifact from TypeScript.
- Claim compatibility with Vercel's private Gateway server or request variants not emitted by the registered public client.
- Add provider recordings, synthetic provider payloads, or entries under provider conformance fixture directories.
- Reintroduce the retired tolerant transport or make provider-domain JSON marshalers protocol authority.

## Decisions

### Establish the AGPL Gateway boundary before runtime code

All Gateway-owned protocol authority and contract evidence lives below `ai-gateway/`, whose nearest license is the canonical AGPLv3 text and whose module path is `github.com/grafana/ai-sdk/ai-gateway`. The root license remains Apache-2.0. Root and Gateway documentation state contribution scope, registered-client provenance, applicable Apache notice obligations, and the required pre-merge and pre-deployment legal confirmation without claiming prior grants are revoked.

The dependency boundary is one-way. The Gateway may later import explicitly pinned SDK modules, but no module or Go source outside `ai-gateway/` may import, require, or replace it. The Gateway module is intentionally absent from `go.work`; a blocking repository check inspects module files and imports and builds/tests the root SDK with `GOWORK=off`.

Alternative: defer the boundary until executable server code lands. Rejected because the production protocol schema and Gateway-owned contract tests are already product artifacts and would otherwise acquire the root Apache license by location. Alternative: add the Gateway module to `go.work`. Rejected because workspace resolution could hide a forbidden SDK-to-Gateway dependency and would weaken independent root builds.

### Use the installed registered packages as executable authority

`ai-gateway/test/providerwire-v4` will invoke the public `createGateway` API from exact AI SDK versions matching `test/conformance/upstream.yaml`. Request capture will use the client's injected `fetch`; tests will not copy the client serializer or construct expected HTTP requests through a second implementation. Baseline validation will include this package manifest, and parity upgrades must update it with every other registered consumer.

The matching upstream commit is used to explain behavior and design cases, but tests execute the published package versions because those are the declared consumer contract.

Alternative: import source from a mutable local upstream checkout. Rejected because checkout state can differ from the registered package baseline. Alternative: reproduce `GatewayLanguageModel.getArgs` locally. Rejected because that would test a duplicate rather than the real client.

### Keep the production schema hand-authored and runtime-neutral

`ai-gateway/providerwire/v4/schema/request.json` will be a draft 2020-12 JSON Schema and the sole production request-shape artifact introduced here. It will describe the complete serialized request projection, including capabilities that later runtimes initially reject as unsupported.

Finite protocol-owned objects will be closed and role/tagged unions will be explicit. The root requires `prompt`; `abortSignal` is absent; integer controls are integers; continuous controls are numbers; required empty strings and arrays remain valid; typed null is accepted only where the registered JSON type permits it; `headers` contains serialized string values; each provider-options namespace is an object with opaque nested JSON; and schema-valued fields remain opaque JSON Schema objects rather than recursively redefining JSON Schema. Provider-reference maps accept provider-name string entries but forbid the reserved `type` property, while provider-tool `id` and custom-part `kind` require the baseline `${string}.${string}` shape.

The schema will be compiled and exercised from TypeScript with a draft 2020-12 validator. Focused positive and negative cases will cover each finite branch and closure rule. Raw runtime handling remains owned by the later unary package: request bytes bound work, invalid raw UTF-8 is rejected, and standard Go JSON plus complete-schema and typed-scalar decoding deliberately use last-value duplicate-member semantics and U+FFFD replacement for escaped lone surrogates.

Alternative: generate JSON Schema from TypeScript. Rejected because generated schemas can silently weaken union closure, presence, or opaque JSON semantics and would couple production authority to a tooling translation. Alternative: initially schema only the text subset. Rejected because unsupported-capability handling in the next work package requires distinguishing schema-invalid input from valid but not-yet-executable input.

### Use compile-time witnesses for every finite surface

The workspace will contain exhaustive `satisfies` maps or exhaustive switches for:

- `keyof Omit<LanguageModelV4CallOptions, "abortSignal">`;
- prompt roles and role-specific request content discriminators;
- file-data arms, tool-result output arms, and approval-response arms;
- function/provider tool kinds and tool-choice kinds;
- generate-result content types and the nested URL/document `sourceType` discriminator;
- stream-part discriminators;
- warning and finish-reason values.

A package upgrade that changes a finite key or discriminator must fail typechecking until the schema, cases, and compatibility assessment are reviewed. These witnesses remain ordinary TypeScript source; they do not generate a classifier or declare runtime support.

Alternative: infer coverage from schema properties at runtime. Rejected because schema inspection does not prove TypeScript union exhaustiveness and cannot reliably represent role-specific type relationships.

### Capture semantic requests, not incidental bytes

A capture fetch will record each call's method, normalized relative path, final case-insensitively normalized headers, streaming mode, parsed JSON body, and request order. It will reject unexpected or repeated effective protocol headers. Expected JSON object member order, user agent values, content length, and other transport volatility will not be golden authority; array and multi-request order will remain significant.

Collision-free cases will assert that the final model header equals the model ID supplied to `createGateway`. A dedicated collision case will seed the canonical lowercase model header in configured headers and provide a case-variant call header, then record the registered client's actual final normalized value. The contract will not reconstruct the model object's original ID from source ordering: a server can observe only the final emitted header and must treat that value as the requested identity.

The request cases will be compact families:

1. unary scalar values, omission, integer zero, finite zero, false, empty strings, empty arrays/objects, and opaque nested provider JSON;
2. comprehensive roles, content parts, files, tools, results, approvals, response format, and provider-option unions;
3. streaming envelope and mode;
4. body-header duplication, ordinary outer-header precedence, and the case-variant protocol-header collision;
5. an ordered multi-call sequence only where it proves behavior not shown by individual requests.

The comprehensive cases will use the real client to prove URL string serialization and `Uint8Array` conversion in top-level file, reasoning-file, and tool-result file positions. Schema cases will separately prove the reserved provider-reference key and dotted provider-tool/custom-kind constraints. Every committed body must validate against the production schema.

Alternative: snapshot raw `Request` bytes and every browser/Node header. Rejected because object member ordering and runtime-added transport headers are not protocol semantics. Alternative: maintain a large example per union arm. Rejected because reviewability is better served by compact comprehensive families plus focused schema cases and compile-time witnesses.

### Separate authored cases, committed goldens, and update behavior

Authored call-option cases are test input. The real client emits candidate semantic captures. Normal tests regenerate those captures in memory and compare them with committed goldens without writing files. A dedicated explicit update command rewrites only the request golden files, then tests revalidate them against the production schema.

The update command will not generate or modify `request.json`, compile-time witnesses, response probes, baseline metadata, or package pins. This keeps a schema change and an upstream package upgrade explicit review decisions rather than update-script side effects.

Alternative: update snapshots automatically during normal checks. Rejected because drift could bless itself in CI or a developer check. Alternative: generate the production schema together with goldens. Rejected because client examples cannot establish complete strict validation.

### Treat client response probes as bounded compatibility evidence

Unary probes will return a representative complete result and assert successful consumption while explicitly showing that server-emitted `request`, `response`, and `warnings` are overwritten by client-owned values. SSE probes will cover clean EOF after a final part, optional `[DONE]`, raw-part suppression/inclusion, response-metadata timestamp conversion, and representative non-2xx JSON error conversion.

The probes will use in-memory `Response` objects through injected fetch. They will not validate future server DTO completeness, safe-error policy, lifecycle ordering, privacy, response bounds, or canonical identity. Those remain requirements of later Go runtime changes and raw HTTP integration.

Alternative: use permissive client acceptance as the server response schema. Rejected because the client uses `z.any()` and masks some unary server fields, so acceptance would permit private or malformed output that the strict server must never emit.

### Add one aggregate non-mutating ProviderWire check

The workspace will expose focused scripts for typechecking, schema/case validation, golden comparison, and client-consumption tests. `mise run test-providerwire-v4` will install the test workspace dependencies and run the complete non-mutating contract signal. `mise run update-providerwire-v4-goldens` will be the only golden-writing task.

Baseline validation will inspect the new package manifest, and `mise run parity-check` will include the non-mutating ProviderWire task. The parity coverage map will classify this as frontend/protocol contract evidence, distinct from provider conformance and from the not-yet-implemented Go replay.

Alternative: fold these cases into `test/conformance/tools`. Rejected because provider conformance owns provider payload provenance and Go-vs-upstream provider/UI replay, while this workspace owns direct public Gateway-client HTTP evidence.

### Preserve the legacy retirement boundary by versioning the new namespace

The existing retired `gateway/providerwire` Go package remains absent. The new schema lives below the explicit `ai-gateway/providerwire/v4` namespace and does not restore an unversioned package, tolerant codec, handler, or Grafana client. Provider-domain JSON remains non-authoritative.

Alternative: place the schema in the retired package root. Rejected because that would blur strict V4 artifacts with the deleted unversioned transport and make later imports ambiguous.

## Risks / Trade-offs

- [The repository boundary is mistaken for retroactive relicensing] → Keep the root Apache license unchanged, state nearest-license scope explicitly, preserve prior grants, and require Grafana legal confirmation before merge or deployment.
- [A workspace or module reference creates a reverse dependency] → Keep `ai-gateway/` out of `go.work`, scan every non-Gateway Go module and source import, and build/test the root module with `GOWORK=off` in blocking CI.
- [The hand-authored schema drifts from TypeScript types] → Combine compile-time finite witnesses, positive/negative branch cases, real-client golden validation, and required baseline review; do not claim mechanical equivalence for open-ended JSON.
- [A compact golden omits an important presence distinction] → Keep a dedicated scalar/presence family and assert absent, false, zero, empty string, empty array, empty object, nested null, and transformed file values explicitly.
- [Permissive response probes overstate server compatibility] → Document and test the unary overwrite behavior, classify probes only as client-consumption evidence, and defer strict output schemas and raw HTTP assertions.
- [Header snapshots become runtime-dependent] → Capture final Fetch-normalized names and values, retain only contract-relevant headers, assert effective protocol-header uniqueness, exclude volatile transport headers, and pin one case-variant collision that documents the registered runtime behavior.
- [The updater accidentally changes authority files] → Restrict its write targets to known golden paths and verify schema, source cases, and package metadata remain untouched.
- [The workspace is omitted during a future parity upgrade] → Add it to baseline validation tests and the parity consumer list before relying on its package pins.
- [New schema tooling increases dependency surface] → Use one established draft 2020-12 validator in the private test workspace, exact-lock it, and keep production Go packages dependency-free in this change.
- [The new directory appears to restore legacy ProviderWire] → Keep the importable unversioned Go package absent, use only the `/v4` namespace, and update the retirement spec and docs with that distinction.

## Migration Plan

1. Establish the `ai-gateway/` module, AGPL-3.0-only nearest license, notices, contribution scope, legal-readiness gate, and blocking one-way dependency verification without changing the root license or workspace graph.
2. Add the versioned schema and private workspace below `ai-gateway/` without changing any runtime route or exported Go API.
3. Register exact baseline dependencies, regenerate only the shared test lockfile path, and add baseline validation coverage.
4. Land committed request goldens only after they are emitted by the registered client and validate against the production schema.
5. Add the non-mutating ProviderWire check to parity verification and classify the new evidence in `test/conformance/PARITY.md`.
6. If rollback is required, remove the new Gateway module/schema/workspace/task registration together; no deployed runtime or consumer migration is involved.

## Open Questions

None. Runtime support classification, strict response schemas, and Go replay are intentionally deferred to work packages 3 and 4.
