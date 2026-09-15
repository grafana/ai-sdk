## 1. Configuration

- [x] 1.1 Add `openai-compatible` provider validation with required `baseURL`, `providerName` restricted to compatible providers, and an unsupported-type error that lists supported types; verify the config table tests and `TestLoadFile_OpenAICompatibleProvider` pass
- [x] 1.2 Update the `anthropic.response-header-timeout` and `anthropic.response-bytes` help text to name both provider types; verify the settings tests pass

## 2. Model construction

- [x] 2.1 Require the pinned `providers/openai-compatible` module in `ai-gateway/go.mod`; verify `GOWORK=off go mod tidy -diff` is clean and the module boundary check passes
- [x] 2.2 Construct compatible models over the shared hardened model client with the resolved key, base URL, provider name and streaming usage; verify the catalog test asserts the chat completions path, bearer credential, backend model ID and `include_usage`
- [x] 2.3 Verify the hardened response byte bound applies to compatible models through the outbound model test

## 3. Real-command evidence

- [x] 3.1 Add a fake compatible backend and command tests for discovery privacy, unary calls and streamed usage; verify `mise run test-ai-gateway-command` passes
- [x] 3.2 Add a command test for sanitized upstream failures and rejected redirects across client errors, logs and metrics
- [x] 3.3 Add a command test proving client abort cancels the backend request while the Gateway stays ready

## 4. Documentation and parity

- [x] 4.1 Update the Gateway README and the `PARITY.md` authenticated service-composition row; verify `mise run validate-parity-baseline` passes
- [x] 4.2 Archive and strictly validate the OpenSpec change

## 5. Validation

- [x] 5.1 Run standalone Gateway tests with race detection on touched packages, build, vet, golangci-lint, the module boundary check and `git diff --check`
- [x] 5.2 Confirm the branch contains only the #120 diff on top of `main` and record the pinned SDK module in the PR description
