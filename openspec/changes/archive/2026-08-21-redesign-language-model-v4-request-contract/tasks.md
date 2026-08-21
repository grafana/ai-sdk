## 1. Evidence and Parent Compatibility Baseline

- [x] 1.1 Confirm `test/conformance/upstream.yaml`, exact source commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`, and installed package source equivalence still match the pinned captures and archived Phase 1 loss analysis.
- [x] 1.2 Before changing provider types, generate the historical request compatibility corpus with parent commit `32e5ab7f1ab9e524477cc0ece04c690a89854a24`; commit metadata, canonical bytes, migration projections, and each parent decode success/projection or exact rejection outcome.
- [x] 1.3 Cover finite parent-encoder equivalence partitions including mixed tool/content inactive fields and valid non-string-valued reference JSON; define byte stability over every parent-encoder-accepted value and decoder compatibility only over the parent-decodable subset.
- [x] 1.4 Convert every Phase 1 loss witness into a failing positive target assertion covering its named provider-model distinction before implementing the corresponding change.

## 2. Exact Provider Request Domain

- [x] 2.1 Implement `LanguageModelNumber` with the specified integer/float constructors, exact accessors, integer canonicalization, invalid zero value, and NaN/infinity rejection.
- [x] 2.2 Add compatibility JSON tests proving exact large-integer bytes, finite fraction handling, invalid input rejection, and non-authoritative codec status.
- [x] 2.3 Change `MaxOutputTokens`, `TopK`, and `Seed` to `*LanguageModelNumber`; change `IncludeRawChunks` and the evidenced response-format/function-tool optional scalars to pointers.
- [x] 2.4 Add `ContentPart.FilePartFilename`, `FilePartWithFilename`, and request-boundary mixed filename validation while preserving `ContentPart.Filename string` for generated response files and sources.
- [x] 2.5 Implement arm-aware, request-directional `ContentPart` compatibility JSON: request files round-trip `FilePartFilename` presence through `filename`, sources retain `Filename`, generated file encoding preserves historical bytes but decode normalizes into `FilePartFilename`, and both fields populated returns an error; test request, generated-file normalization, and source cases separately.
- [x] 2.6 Change the evidenced approval reason, tool-result filename, execution-denied reason, and request-side `ProviderExecuted` fields to pointers with absent versus explicit empty/false coverage.
- [x] 2.7 Implement the exact public `DataContentType`, constants, existing fields, five constructors, and `DataType` resolver, including copied inputs and valid empty data, URL, reference-object, and text semantics.
- [x] 2.8 Update `DataContent` validation and compatibility JSON methods for private empty-arm selection, deterministic legacy inference of existing non-empty response literals, data conflicts, inactive payloads, and strict reference shape without changing response codecs.
- [x] 2.9 Retain and reframe provider request custom marshalers plus `upstream_*_compat_test.go` as compatibility-only behavior; rename `call_options_wire_test.go` so it no longer claims protocol authority.

## 3. Root Orchestration, Fallback, and Middleware

- [x] 3.1 Update root call configuration and prepare-step values to carry `LanguageModelNumber` and raw-chunk presence, constructing existing integer options through `LanguageModelNumberFromInt`.
- [x] 3.2 Update prompt, tool, file, approval, and execution-denied producers to construct each changed optional field by source semantics, including `FilePartWithFilename` for present request filenames.
- [x] 3.3 Update `ToResponseMessages` to move generated-response `Filename` into request `FilePartFilename`, preserve an existing request pointer including explicit empty, clear output `Filename`, and cover invalid mixed producer state.
- [x] 3.4 Update Anthropic server-tool citation tracking to read prompt `FilePartFilename`, preserve absent/explicit-empty fallback behavior, reject mixed request state at conversion, and keep emitted source filenames in response/source fields.
- [x] 3.5 Add root regression tests for exact large integers, fractions where directly accepted, absent versus explicit false/empty values, empty selected file data, system-message preservation, and nil versus non-nil empty collections.
- [x] 3.6 Update fallback and default-setting/copy/merge middleware to forward exact number variants, changed pointers, and collections without normalization or aliasing regressions.
- [x] 3.7 Update logger, Prometheus, Agent Observability, and other request inspection to avoid converting large integers through `float64`, truncating fractions, or inventing presence; logger privacy sanitization SHALL clear both filename fields, and observability media mapping SHALL read the direction-specific field.
- [x] 3.8 Update tool-description and other request-mutating middleware to preserve nil versus explicit empty values while retaining existing non-empty behavior.
- [x] 3.9 Complete the finite filename producer/consumer inventory from the design, update its matching tests, and classify every remaining repository `ContentPartTypeFile`/`FilePart`/`ToolResultContentValue`/`Filename` occurrence as prompt request, tool-result request, generated response, source, or unrelated stream/response state.

## 4. Exact Provider Request Conversion

- [x] 4.1 Add focused final serialized-request tests and request snapshots for supported forwarded fractions, conditional model/reasoning omission, warnings, arithmetic and clamping, exact integral behavior, and a large historical integer path; Anthropic/Vertex coverage SHALL include fractional `topK` supported, thinking enabled, and sampling-rejecting model cases.
- [x] 4.2 Update Anthropic and Vertex Anthropic to forward supported `maxOutputTokens` and `topK` exactly through SDK extra-field overrides when required, and explicitly remove both generated and override `top_k` state on unsupported-model or thinking paths while preserving pinned model caps, reasoning-budget arithmetic/clamping, and seed warnings.
- [x] 4.3 Update Bedrock's repository-owned request representation to forward supported `maxOutputTokens` and `topK` exactly while preserving pinned Anthropic-thinking budget arithmetic, `topK` omission/warnings, and seed warnings.
- [x] 4.4 Update OpenAI Responses to forward `maxOutputTokens` exactly through the generated field or SDK extra-field override while preserving pinned `topK` and seed warnings.
- [x] 4.5 Update OpenAI-compatible to forward `maxOutputTokens` and seed exactly through its map-based body while preserving pinned `topK` warnings.
- [x] 4.6 Verify the final semantic provider requests match exact pinned source behavior; stop for an explicit intentional-deviation decision if any SDK override or request representation cannot do so.
- [x] 4.7 Update OpenAI Responses custom-tool continuation file conversion so nil `ToolResultContentValue.Filename` defaults to `"data"`, explicit empty remains empty, and non-empty remains exact; cover all three states at the final serialized request.
- [x] 4.8 Add selected-role/arm and filename-direction validation plus focused invalid-state tests to each direct provider request mapper; legacy adapter acceptance SHALL NOT substitute for direct provider validation.
- [x] 4.9 Update provider conformance request assertions only for intentional semantic changes while leaving provider inputs, response expectations, and UI expectations unchanged.

## 5. Request-Only Tolerant Legacy ProviderWire

- [x] 5.1 Introduce a complete private request-only legacy representation and explicit top-level `CallOptions`, response-format, tool, and tool-choice mapping in `gateway/providerwire`, retaining parent `omitempty` collection emission while preserving explicitly present empty members during decode.
- [x] 5.2 Implement explicit message/content, file/source filename migration, system-message, tool-result, provider-option, collection-presence, and selected file-data mapping without direct provider-struct marshaling; reproduce parent field emission and `DataContent` precedence for every parent-encoder-accepted shape.
- [x] 5.3 Encode exact integer and finite floating `LanguageModelNumber` variants independently; reject invalid zero values, non-finite values, and redesigned values without a legacy representation, but preserve parent-encodable mixed inactive fields and arbitrary valid-JSON references.
- [x] 5.4 Preserve tolerant upstream/legacy request decode behavior while mapping supported redesigned distinctions into provider-domain pointers, numbers, and discriminators.
- [x] 5.5 Prove each corpus migration projection equals its parent-produced bytes; use recorded parent semantic projections only for successful parent-decode rows and retain recorded rejection rows without claiming decoder compatibility.
- [x] 5.6 Add separate round-trip coverage for newly representable fractions, explicit false/empty scalars, request filenames, and empty file-data arms without labeling them historical compatibility; cover explicit empty collection decode separately and retain parent-collapsed encode behavior.
- [x] 5.7 Keep generate-result, SSE, API-call-error, and stream wire behavior unchanged; minimally update `legacyToolResult` so nil and explicit-empty `*Reason` still emit historical `result:""` rather than null, with focused absent/empty/non-empty tests.
- [x] 5.8 Update handler tests to prove redesigned request values and parent-permissive legacy states reach model dispatch, while adapter-intrinsic decode errors bypass model resolution; direct-provider selected-arm rejection occurs after resolution in provider conversion and SHALL NOT be attributed to tolerant handler validation.

## 6. Grafana Transport and Evidence Lifecycle

- [x] 6.1 Update the Grafana hosted client to pass exact numbers and presence-aware values through `providerwire.EncodeCallOptions` without conversion or duplicate DTOs.
- [x] 6.2 Extend Grafana client/server tests for redesigned request values, exact parent request bytes, unary transport, ordered streaming, and current decoding of the parent corpus.
- [x] 6.3 Retire `test/providerwire-v4/phase2-delta.md` after preserving its rationale in the archived Phase 1 change, its row-level detail in repository history, and its durable strict-codec responsibilities in canonical specifications.
- [x] 6.4 Convert `provider/providerwire_v4_loss_test.go` into stable positive external-package assertions in `provider/request_contract_external_test.go` covering every resolved provider-model distinction.
- [x] 6.5 Remove the transitional markdown-to-test-name coupling and keep `check-providerwire-v4` focused on non-mutating pinned-client evidence validation while normal Go workflows run the provider package.
- [x] 6.6 Update the ProviderWire V4 README and `test/conformance/PARITY.md` from active-loss language to resolved positive provider-contract coverage, parent-pinned legacy evidence, and unchanged strict-runtime gaps.
- [x] 6.7 Run the ProviderWire V4 evidence check without regenerating pinned captures or classification; update generated evidence only if exact registered client observations independently changed.

## 7. Documentation and Validation

- [x] 7.1 Update godoc and affected canonical specification purpose text to document `LanguageModelNumber`, exact `DataContent`, request/generated-response/source filename ownership, arm-aware compatibility JSON, and tolerant-parent versus strict-mapper validation boundaries.
- [x] 7.2 Run focused root and nested provider tests, including race coverage where repository tasks provide it, and fix all regressions.
- [x] 7.3 Run `mise run check-providerwire-v4`, `mise run validate-parity-baseline`, `mise run parity-check`, `mise run test-conformance`, `mise run test-integration`, and `mise run test-interop`.
- [x] 7.4 Run `mise run test`, `mise run vet`, `mise run lint`, `mise run lint-docs`, `openspec validate --all --strict`, and `git diff --check`.
- [x] 7.5 Verify implementation against the proposal, design, delta specs, archived Phase 1 loss analysis, parent compatibility corpus, and exact pinned upstream sources; document changed artifacts, commands, and residual risks.
- [x] 7.6 Synchronize completed delta specs into canonical specifications.
- [x] 7.7 After explicit user approval, archive with `openspec archive redesign-language-model-v4-request-contract --skip-specs` because canonical specifications are already synchronized, then confirm zero active OpenSpec changes.
