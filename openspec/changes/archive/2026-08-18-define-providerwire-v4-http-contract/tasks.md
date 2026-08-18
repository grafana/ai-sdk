## 1. Baseline and Contract Scaffold

- [x] 1.1 Recheck the live prerequisite behavior, clean worktree, registered source commit, exact npm pins, and relevant commit-to-release source equivalence; stop and replan on parent or baseline drift.
- [x] 1.2 Create the contract-only `gateway/providerwire/v4` package documentation, OpenAPI/schema directories, contract testdata layout, and interop fixture index without adding production handler, decoder, client, adapter, or DTO APIs.
- [x] 1.3 Add the pinned test-only `@redocly/cli@2.46.1` and `github.com/go-json-experiment/json@v0.0.0-20260623181947-01eb4420fa68` dependencies and update lock/module metadata without enabling repository-wide JSON experiments.
- [x] 1.4 Use `test/conformance/upstream.yaml` as the baseline authority and record generated-versus-curated fixture ownership in the interop index.

## 2. Serialized Contract Inventory

- [x] 2.1 Inventory every post-serialization request field, message role, content arm, tool/tool-choice arm, response-format arm, file-data arm, and tool-result arm from the exact pinned packages, including undefined omission and explicit-empty behavior.
- [x] 2.2 Inventory every generate-result object and content arm, including required top-level fields, usage/finish-reason undefined omission, warnings, request/response values, metadata maps, and timestamp serialization.
- [x] 2.3 Inventory every stream-part arm and safe error field, including exact discriminator membership, generated-file restrictions, source variants, non-null streamed tool results, raw values, and retryability semantics.
- [x] 2.4 Encode the field-level inventory directly in the normative schemas and keep cross-cutting open-boundary and representability rules in the capability specification.

## 3. Pinned Stock-Client Capture Harness

- [x] 3.1 Implement a deterministic TypeScript recording server that is independent of both provider-wire handlers and stores method/path, allowlisted normalized headers, and semantic JSON with ownership recorded in the fixture index.
- [x] 3.2 Add direct `doGenerate` and `doStream` capture scenarios that prove the pinned method, path, routing headers, body transformations, and unary/streaming selection.
- [x] 3.3 Add orchestration-level `generateText` and `streamText` capture scenarios so `ai@7.0.65` request construction is executable evidence rather than inferred from provider types.
- [x] 3.4 Capture all message roles, function/provider tools, tool choice, client/provider-executed tool flows, prompt files, reasoning files, and nested tool-result file data, including `Uint8Array` base64 conversion.
- [x] 3.5 Capture structured response formats, opaque null where allowed, body headers, provider options and representative Gateway controls, raw-chunk intent, explicit empty collections, and configured/call/model/observability header collisions.
- [x] 3.6 Implement temporary-directory recapture and semantic comparison for normal verification plus a separate explicit fixture-update command.
- [x] 3.7 Enforce capture ownership and privacy by excluding credential values, volatile identifiers, full user agents, machine-local data, and provider-recording claims from committed fixtures.

## 4. Curated JSON Schema 2020-12 Contracts

- [x] 4.1 Define stable offline `$id` resources and shared `$defs` for JSONValue versus JSONObject, provider option/metadata maps, possibly empty provider references without a `type` member, exact warnings/usage/finish metadata, and reusable selected arms.
- [x] 4.2 Author `schema/request.json` for the complete post-Gateway request projection with closed standard objects, exact role/union arms, no `abortSignal`, string-only serialized headers, tagged file data, and preserved explicit empties.
- [x] 4.3 Author `schema/generate-result.json` for every pinned content arm and exact result metadata, including required content/finish/usage/warnings, undefined-field omission, RFC 3339 timestamps, and structurally representable opaque values.
- [x] 4.4 Author `schema/stream-part.json` for every pinned stream discriminator with exact full arms, constrained generated files, exact source variants, non-null streamed tool results, and raw/error opaque values.
- [x] 4.5 Author `schema/error.json` as exact full arms for the seven wire-recognized Gateway types, HTTP-correlated `statusCode`, explicit `isRetryable`, only public-model `modelId` or `ruleId` params on their matching arms, no client-originated types or `code`, and no backend debugging fields.
- [x] 4.6 Add a complete positive payload corpus that covers every request/result/stream arm, open boundary, explicit-empty distinction, response projection, and error retryability override.
- [x] 4.7 Add local negative schema fixtures for unknown fields, inactive siblings, missing/null/wrong types, role violations, invalid file/reference values, scalar provider namespaces, null streamed results, Go-only fields, and unsafe error fields.

## 5. Strict JSON Syntax Evidence

- [x] 5.1 Implement a contract-test/tooling-only `jsontext.Decoder` wrapper that reads exactly one raw value, requires EOF after trailing whitespace, and passes original bytes onward unchanged.
- [x] 5.2 Add syntax fixtures for nested and escaped-equivalent duplicates, trailing values, truncation, malformed escapes, invalid raw UTF-8, lone high/low surrogates, valid surrogate pairs, and trailing whitespace.
- [x] 5.3 Prove syntax failures are classified before schema failures and prove no production HTTP path imports or calls the H1 syntax tooling.

## 6. OpenAPI and Contract Validation

- [x] 6.1 Author `openapi.yaml` for only `POST /language-model`, the three required routing headers, mandatory JSON request media type, HTTP 200 unary JSON success, HTTP 200 streaming SSE success, and non-2xx safe JSON errors with local schema references.
- [x] 6.2 Add focused envelope fixtures/tests for exact routing values, model-ID whitespace, content-type parameters, omitted/invalid content type, omitted/compatible/incompatible Accept, malformed ranges, wildcard matching, and `q=0`.
- [x] 6.3 Implement a protocol-local offline schema registry that loads every resource before Draft 2020-12 compilation and rejects unresolved or network references.
- [x] 6.4 Implement the contract corpus runner so each positive validates and each negative fails at its declared envelope, syntax, or schema stage with an expected stable category and safe instance path.
- [x] 6.5 Configure Redocly to lint and bundle the complete OpenAPI 3.1 document offline, and test that all external payload references resolve without treating OpenAPI as SSE lifecycle authority.

## 7. Pinned Response-Consumption Evidence

- [x] 7.1 Add locally labeled stock-client consumption projections for unary JSON, exact `data: <JSON>\n\n` stream event framing without `event:`, final-event-before-EOF, clean EOF, and tolerated `[DONE]` without classifying `[DONE]` as protocol payload.
- [x] 7.2 Add paired stream projections proving raw-part filtering follows `includeRawChunks` and RFC 3339 response-metadata timestamps become dates.
- [x] 7.3 Add representative non-2xx projections proving the pinned client recognizes the safe nested error shape and documenting its HTTP-status retry inference separately from explicit wire retryability.
- [x] 7.4 Add retryable-400 and non-retryable-500 fixtures that preserve explicit booleans for the future Go client while remaining consumable by the pinned stock client.

## 8. Tasks and Parity Metadata

- [x] 8.1 Add `mise run validate-providerwire-v4-contract`, `mise run test-interop-contract`, and a separate explicit capture-update task with non-mutating verification defaults.
- [x] 8.2 Extend baseline-consumer validation and tests as needed so every new TypeScript AI SDK import remains pinned to `test/conformance/upstream.yaml`.
- [x] 8.3 Add a separate ProviderWire V4 HTTP contract row to `test/conformance/PARITY.md` that claims request emission, curated schema validation, and response consumption only while preserving legacy transport classifications.
- [x] 8.4 Leave user-facing provider-wire documentation unchanged until a V4 runtime capability is implemented.
- [x] 8.5 Run existing legacy package, Grafana, interop, and frontend checks without adding source-shape assertions to the V4 contract package.

## 9. Validation and OpenSpec Completion

- [x] 9.1 Run formatting, TypeScript type checking, `mise run validate-providerwire-v4-contract`, and `mise run test-interop-contract`; resolve all failures without weakening exact union or syntax rules.
- [x] 9.2 Run `go test -race ./gateway/providerwire/...`, the existing `mise run test-interop`, and relevant Grafana provider tests to prove legacy coexistence.
- [x] 9.3 Run `mise run validate-parity-baseline` and `mise run parity-check`; classify every observed difference as pinned-client behavior, local serialized projection, host restriction, Go adaptation, defect, or coverage gap.
- [x] 9.4 Run `git diff --check` and privacy scans for secrets, machine-local paths, unrelated branches or plans, mislabeled provider recordings, and accidental production V4 decoder/handler/client code.
- [x] 9.5 Validate the change strictly, verify implementation against all artifacts, synchronize the three capability specs, archive the change, confirm zero active changes, and run `openspec validate --all --strict`.
