## 1. OpenAI producer contract and regression tests

- [x] 1.1 Review and approve the exact exported model capability option and typed per-call option names against the pinned `@ai-sdk/openai` 4.0.71 APIs/tests before changing the public Go surface; retain provider-only scope and tri-state behavior.
- [x] 1.2 Add focused OpenAI request tests (`providers/openai/prepare_tools_test.go` or adjacent) for default and explicit true/false per-call values, enabled/disabled model capability, disabled capability plus per-call true, both web tool IDs, absence of a web tool, and explicit `Include` preservation under both disable switches; assert web tool emission and no duplicate include.
- [x] 1.3 Implement default-enabled model capability and nullable per-call `includeWebSearchSources` handling in `providers/openai/options.go`, `model.go`, `convert_request.go`, `apply_options.go`, shared by `DoGenerate` and `DoStream`; do not gate explicitly requested includes or unrelated include kinds.
- [x] 1.4 Assert unchanged code-interpreter outputs, positive logprobs and stateless encrypted-reasoning includes in focused request tests; run `cd providers/openai && go test ./...` and standalone `GOWORK=off go test -mod=readonly ./...` from that module.

## 2. Candidate-source producer and Bedrock consumer

- [x] 2.1 Keep OpenAI producer and Bedrock consumer source independently testable while retaining existing merged-main dependency pins; use #262's candidate-source checks to validate the coordinated change.
- [x] 2.2 Add Mantle strict fake-endpoint tests in `providers/bedrock/mantle/provider_test.go` for `DoGenerate` and `DoStream`: reject unsupported automatic source include, assert web tool survives, consume the accepted SSE stream and assert identity; separately inspect explicit caller include serialization without claiming acceptance by Mantle.
- [x] 2.3 Configure `providers/bedrock/mantle/provider.go` to disable only automatic web source inclusion and keep `providers/bedrock/go.mod`/`go.sum` on merged-main pins; verify no unrelated #207, #164 or Chat behavior changes.
- [x] 2.4 Run `cd providers/bedrock && go test ./...` against the candidate SDK workspace and `mise run verify-merged-pins`; reserve `GOWORK=off` Bedrock validation for a published OpenAI producer before Bedrock artifact publication.

## 3. Parity and evidence review

- [x] 3.1 Inspect existing provider request snapshots for affected cases; only regenerate expectations from provenance-valid captured or imported inputs when necessary, never synthesize recorded/upstream provider input chunks; document a provider-boundary coverage gap if no suitable fixture exists.
- [x] 3.2 Run `mise run parity-check` for changed parity behavior plus required candidate-source checks; report pinned-source alignment, synthetic endpoint evidence versus live Mantle acceptance, and the separate post-merge standalone-module validation boundary without altering registered upstream pins.
