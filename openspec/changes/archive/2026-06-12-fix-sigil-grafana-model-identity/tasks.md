## 1. Test Coverage

- [x] 1.1 Add a generate-path Sigil recording test where a `grafana` wrapper model returns response metadata with `Provider: "anthropic"` and assert the recorded generation model uses `anthropic`.
- [x] 1.2 Add a generate-path test proving incomplete response metadata keeps the seed model identity and does not add transport metadata.
- [x] 1.3 Add a stream recorder test where `PartResponseMeta` supplies backend provider/model and assert `Generation().Model` uses that backend identity.
- [x] 1.4 Add a stream recorder test proving incomplete `PartResponseMeta` keeps the seed identity.
- [x] 1.5 Add direct-provider coverage proving matching response identity does not emit `ai_sdk.transport.*` metadata.

## 2. Generation Mapping

- [x] 2.1 Introduce internal model identity helpers in `middleware/sigil` for comparing seed and response model identities and applying transport metadata.
- [x] 2.2 Update generate result mapping to set `sigilsdk.Generation.Model` from `GenerateResult.Response.Provider` and `ModelID` when both are populated.
- [x] 2.3 Preserve the existing exported `MapGenerateResult` behavior or update the public API/spec consistently if a signature change is required.
- [x] 2.4 Ensure generated metadata merges transport keys without overwriting caller-provided `ContextInfo.Metadata` unexpectedly.

## 3. Stream Recording

- [x] 3.1 Extend `StreamRecorder` state to store response provider, model ID, and response ID observed from `PartResponseMeta`.
- [x] 3.2 Update `StreamRecorder.Observe` to handle `provider.PartResponseMeta` without affecting payload/first-token timing.
- [x] 3.3 Update `StreamRecorder.Generation()` to apply response model identity and transport metadata consistently with generate mapping.

## 4. Documentation and Verification

- [x] 4.1 Update Sigil documentation to mention backend response identity and `ai_sdk.transport.*` metadata for gateway-style providers.
- [x] 4.2 Run `go test ./...` in `middleware/sigil`.
- [x] 4.3 Run root `go test ./...` if root code or provider types are changed.
- [x] 4.4 Run `openspec status --change "fix-sigil-grafana-model-identity"` and confirm the change is apply-ready.
