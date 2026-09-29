## 1. Configuration

- [x] 1.1 Accept `type: openai` with an optional `baseURL`; verify `TestLoadFile_OpenAIProviderBaseURLOptional` and the unsupported-type row pass
- [x] 1.2 State that the shared model limit flags govern all provider types in their help text; verify the settings tests pass

## 2. Model construction

- [x] 2.1 Require the pinned `providers/openai` module and `openai-go/v3` in `ai-gateway/go.mod`; verify `GOWORK=off go mod tidy -diff` is clean and the module boundary check passes
- [x] 2.2 Construct Responses models through `NewResponsesWithClient` over explicit options with the production base URL default; verify the poisoned-environment catalog test asserts the default URL, bearer credential, absent organization, project and custom headers, and backend model ID
- [x] 2.3 Verify the hardened response byte bound applies to OpenAI Responses models through the outbound model test

## 3. Real-command evidence

- [x] 3.1 Add a fake OpenAI Responses backend and a command test for discovery privacy, unary calls and streamed usage; verify `mise run test-ai-gateway-command` passes

## 4. Documentation and parity

- [x] 4.1 Update the Gateway README and the `PARITY.md` authenticated service-composition row; verify `mise run validate-parity-baseline` passes
- [x] 4.2 Archive and strictly validate the OpenSpec change

## 5. Validation

- [x] 5.1 Run standalone Gateway tests with race detection on touched packages, build, vet, golangci-lint, the module boundary check and `git diff --check`
