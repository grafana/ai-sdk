## 1. Baseline evidence and approval gates

- [ ] 1.1 Confirm exact registered OpenAI 4.0.72 source/tests and inspect the available Go SDK response/input unions; classify current request and metadata coverage in `test/conformance/PARITY.md`, and determine whether an existing provenanced provider request fixture can be extended by approved recording/import. If not, plan focused fake-HTTP tests without fabricated `recorded/` or `upstream/` inputs.
- [ ] 1.2 Obtain owner approval for exported update/trigger/tool async option names, typed enum/validation, optional-bool and metadata representation, and Mantle/Azure endpoint scope before changing the public API; reconcile any changed contract with this spec before coding.
- [ ] 1.3 Test existing root `streamtext.go`/`GenerateText` metadata, caller and approval projection against exact upstream core behavior. If additional execution or provider-contract changes are necessary, coordinate and approve a narrowly scoped design and independently green published-module sequence before implementation; do not silently add an async scheduler.

## 2. Provider request construction

- [ ] 2.1 Write deterministic `providers/openai` model-matrix and `DoGenerate`/`DoStream` request assertions for supported/unsupported resolved reasoning efforts, including core fallback, aliases, third-party IDs and existing endpoint overrides; implement the capability flags and warning/omission behavior in `models.go`, `convert_request.go`, and `apply_options.go`.
- [ ] 2.2 Write request assertions for supported and rejected `reasoningEffortUpdate` combinations (including present empty context management), request-level effort independence, previous-response continuation, `compactionTrigger` true/false and input order/immutability; add approved typed options and encode controls via pinned SDK input union without modifying prompt history.
- [ ] 2.3 Write tool request assertions for absent/false/true async on ordinary and namespaced functions and `openai.custom`, supported and unsupported models, preserved allowed callers and tool-named warnings; implement in `prepare_tools.go` without mutating tool args.

## 3. Metadata and continuation

- [ ] 3.1 Add synthetic generate and stream function/custom call tests for present true/false/absent async metadata, stream added/done precedence and fallback, item ID, namespace and caller retention; map SDK JSON presence in `convert_response.go` and `stream_adapter.go` to namespaced provider metadata.
- [ ] 3.2 Test the next request reconstructed from generated/streamed call metadata with `store: false` and with stored custom references versus inline function calls; add `OpenAIPartOptions` mapping and `convert_provider_tool_continuation.go` conversion while retaining existing `call_id` pairing.
- [ ] 3.3 Add tests for programmatic `execution-denied` results identified by result caller or matching assistant call, direct denied results, provider-executed synthetic denials, and no HTTP request on unsupported continuations; implement targeted conversion validation in `convert_messages.go`/`convert_provider_tool_continuation.go`.
- [ ] 3.4 Exercise `GenerateText`, `StreamText`, and resumed multi-step tool loops with async/caller metadata and denied programmatic continuations. Add a deterministic `test/integration/testserver` async-metadata scenario and matching Vitest `parseJsonEventStream`/`uiMessageChunkSchema` test asserting true/explicit false/absent metadata on chunks and assembled continuation messages. Change root/provider contracts only if this proof exposes a real gap and owner-approved design covers it.

## 4. Validation and delivery

- [ ] 4.1 Run relevant provider and root tests, `mise run test-integration` for the required async-metadata scenario, `mise run parity-check`, and build/vet as applicable; update only legitimate provenanced request expectations and stable coverage/evidence boundaries, not issue catalogs or frozen pins.
- [ ] 4.2 Verify any root dependency is published and the OpenAI module builds/tests independently with `GOWORK=off` against a publicly resolved compatible pin (or deliver provider-only if root changes are unnecessary); record residual lack of live-provider evidence rather than claiming API acceptance from fake transport tests.
