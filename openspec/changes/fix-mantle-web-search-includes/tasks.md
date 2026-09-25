## 1. OpenAI producer contract and regression tests

- [ ] 1.1 Review and approve the exact exported model capability option and typed per-call option names against the pinned `@ai-sdk/openai` 4.0.71 APIs/tests before changing the public Go surface; retain provider-only scope and tri-state behavior.
- [ ] 1.2 Add focused OpenAI request tests (`providers/openai/prepare_tools_test.go` or adjacent) for default and explicit true/false per-call values, enabled/disabled model capability, disabled capability plus per-call true, both web tool IDs, absence of a web tool, and explicit `Include` preservation under both disable switches; assert web tool emission and no duplicate include.
- [ ] 1.3 Implement default-enabled model capability and nullable per-call `includeWebSearchSources` handling in `providers/openai/options.go`, `model.go`, `convert_request.go`, `apply_options.go`, shared by `DoGenerate` and `DoStream`; do not gate explicitly requested includes or unrelated include kinds.
- [ ] 1.4 Assert unchanged code-interpreter outputs, positive logprobs and stateless encrypted-reasoning includes in focused request tests; run `cd providers/openai && go test ./...` and standalone `GOWORK=off go test -mod=readonly ./...` from that module.

## 2. Published producer to Bedrock consumer

- [ ] 2.1 Make OpenAI producer independently mergeable and publish a publicly resolvable version containing the capability; do not adopt an unpublished or workspace-only version in Bedrock.
- [ ] 2.2 Add Mantle strict fake-endpoint tests in `providers/bedrock/mantle/provider_test.go` for `DoGenerate` and `DoStream`: reject unsupported automatic source include, assert web tool survives, consume the accepted SSE stream and assert identity; separately inspect explicit caller include serialization without claiming acceptance by Mantle.
- [ ] 2.3 Configure `providers/bedrock/mantle/provider.go` to disable only automatic web source inclusion and update `providers/bedrock/go.mod`/`go.sum` to the published OpenAI producer; verify no unrelated #207, #164 or Chat behavior changes.
- [ ] 2.4 Run `cd providers/bedrock && go test ./...` and `cd providers/bedrock && GOWORK=off go test -mod=readonly ./...` to prove the consumer boundary; check include behavior with published dependencies, not just `go.work`.

## 3. Parity and evidence review

- [ ] 3.1 Inspect existing provider request snapshots for affected cases; only regenerate expectations from provenance-valid captured or imported inputs when necessary, never synthesize recorded/upstream provider input chunks; document a provider-boundary coverage gap if no suitable fixture exists.
- [ ] 3.2 Run `mise run parity-check` for changed parity behavior plus relevant build/test checks; report pinned-source alignment, synthetic endpoint evidence versus live Mantle acceptance, standalone published-module results and any residual live verification gap without altering the registered upstream pins.
