## 1. Register the Baseline-Pinned Contract Workspace

- [x] 1.1 Create private `test/providerwire-v4` package and TypeScript configuration with exact registered `@ai-sdk/gateway@4.0.52`, `@ai-sdk/provider@4.0.7`, and `@ai-sdk/provider-utils@5.0.27` dependencies plus the minimal locked test/schema tooling.
- [x] 1.2 Register `test/providerwire-v4` in `test/pnpm-workspace.yaml`, install through the shared test workspace, and update `test/pnpm-lock.yaml` without changing the registered AI SDK baseline.
- [x] 1.3 Add compile-time exhaustive witnesses for call-option keys excluding `abortSignal`, request roles and unions, unary content including nested URL/document `sourceType`, stream parts, warnings, and finish reasons; confirm the workspace typecheck fails for an intentionally incomplete local witness before restoring exhaustive coverage.

## 2. Define and Test the Complete Request Schema

- [x] 2.1 Add the hand-authored draft 2020-12 schema at `gateway/providerwire/v4/schema/request.json` with a closed root, required prompt, scalar settings, response formats, body headers, reasoning, provider options, tools, tool choice, and every registered role/content/data/result/approval union.
- [x] 2.2 Model strict presence and value semantics in the schema, including integer token/count/seed fields, continuous numeric fields, required empty values, forbidden typed null, object-valued provider namespaces with opaque nested JSON, opaque JSON Schema objects, provider-reference maps without a `type` property, and dotted provider-tool `id` and custom-part `kind` values.
- [x] 2.3 Add schema compilation and positive cases covering every registered request branch, explicit false/zero/empty values, nested opaque JSON, provider references, dotted provider-tool/custom identifiers, and all role-compatible unions.
- [x] 2.4 Add negative schema cases for unknown members and discriminators, role-incompatible content, inactive or mixed union arms, malformed provider namespaces, provider references containing `type`, undotted provider-tool IDs or custom kinds, forbidden `abortSignal` and typed null, missing required members, and fractional integer controls.

## 3. Capture Real Gateway Requests and Commit Goldens

- [x] 3.1 Implement an injected capture fetch around the public registered `createGateway` client that records ordered method, relative path, normalized contract headers, mode, and semantic JSON while excluding volatile transport details.
- [x] 3.2 Add the unary scalar/presence request family covering omission, false, integer and floating zero, empty strings/arrays/objects, stop sequences, headers, reasoning, and opaque provider JSON.
- [x] 3.3 Add the comprehensive request family covering every prompt role, content part, file-data arm, function/provider tool shape, tool choice, tool-result output, approval response, response format, and provider-options union, including URL serialization and `Uint8Array` conversion in every client-transformed file position.
- [x] 3.4 Add streaming, outer-header precedence/body-header duplication, and meaningful ordered multi-call cases; assert exact model identity only for collision-free requests, effective protocol-header uniqueness, streaming mode, request order, and cancellation signal omission from JSON.
- [x] 3.5 Add a case-variant collision capture with canonical configured `ai-language-model-id`, a case-variant call header valued `call`, and model ID `actual`; assert the final normalized header is `call` and document that the server can use only that emitted identity.
- [x] 3.6 Implement in-memory regeneration, schema validation, and semantic comparison against committed goldens with focused diffs and no normal-test writes.
- [x] 3.7 Add the explicit golden updater restricted to known request golden paths, run it to create the reviewed baseline captures, and verify every emitted body validates against `request.json`.

## 4. Prove Registered Client Consumption Behavior

- [x] 4.1 Add unary success probes for representative content, usage, finish reason, and response headers, including explicit assertions that client-owned `request`, `response`, and `warnings` overwrite server-supplied values.
- [x] 4.2 Add SSE probes for finish followed by clean EOF, tolerated `[DONE]`, ordered parts, raw-part suppression and inclusion, and response-metadata timestamp conversion to `Date`.
- [x] 4.3 Add representative structured non-2xx probes for both unary and streaming setup, asserting the registered public Gateway error classification, status, and message without treating the accepted body as a future server schema.

## 5. Integrate Contract and Parity Workflows

- [x] 5.1 Add focused workspace scripts for typechecking, schema/golden verification, client-consumption tests, the aggregate non-mutating contract check, and the explicit golden update action.
- [x] 5.2 Extend parity baseline validation and its tests to enumerate `test/providerwire-v4/package.json` and report package/version drift for that workspace.
- [x] 5.3 Add `mise run test-providerwire-v4` and `mise run update-providerwire-v4-goldens`, and include only the non-mutating contract task in `mise run parity-check`.
- [x] 5.4 Update `test/conformance/PARITY.md` to classify registered-client request projection and consumption coverage separately from provider conformance, hook interop, and deferred strict Go runtime evidence.
- [x] 5.5 Verify current documentation and package references distinguish strict `gateway/providerwire/v4` artifacts from the retired unversioned handler/client and do not make a private Vercel server compatibility claim.

## 6. Verify the Change

- [x] 6.1 Run the workspace typecheck, schema/golden tests, client-consumption tests, and aggregate `mise run test-providerwire-v4` task.
- [x] 6.2 Run the explicit updater once from a clean tracked-artifact baseline, then rerun the aggregate check and verify normal verification produces no file changes.
- [x] 6.3 Run `mise run validate-parity-baseline` and `mise run parity-check` and confirm the new workspace is enforced without rewriting committed files.
- [x] 6.4 Run `mise run build`, `mise run vet`, `mise run lint`, and `mise run test` to confirm the schema namespace and workspace registration do not regress retained Go modules or tests.
- [x] 6.5 Search for restored legacy imports/codecs and unintended ProviderWire V4 runtime code; confirm the diff adds no handler, resolver, model invocation path, Go client, provider fixture input, generated runtime classifier, or response schema.
- [x] 6.6 Run `openspec validate establish-providerwire-v4-http-contract --strict` and review the final diff against the registered upstream commit and phase 2 acceptance criteria.
