# Bidirectional upstream-client interop

This harness runs the registered `@ai-sdk/gateway` and `ai` clients against the
Go legacy and strict provider-wire handlers. Its 17 tests prove shared canonical
behavior through both deployments plus the strict handler's HTTP 500 retry
behavior.

## How it works

- `testserver/` serves mock `provider.LanguageModel` implementations through
  the legacy `gateway/providerwire` handler and strict
  `gateway/providerwire/v4` handler. The model ID selects the scenario.
- `global-setup.ts` builds and starts the Go server on an ephemeral port, then
  records distinct gateway base URLs:
  - `http://127.0.0.1:PORT/api/v1/aisdk/legacy`
  - `http://127.0.0.1:PORT/api/v1/aisdk/strict`
- `interop.test.ts` runs eight shared scenarios against both URLs and one
  strict-only retry scenario.

## Run

```bash
mise run test-interop
# or, from this directory:
pnpm test
```

## Scenarios

| Scenario | Coverage |
| --- | --- |
| `generate-rich` | rich unary content, usage, metadata, and privacy |
| `tool-result-file-input` | canonical tool-result file data input |
| `stream-text` | streaming text with system and user prompts |
| `tool-call` | client-executed tool-call round trip |
| `file-input` | upstream file/image input decoding |
| `stream-sources` | URL and document source parts |
| `error-mid-stream` | ordered continuation after a provider error part |
| `error-pre-stream` | pre-stream HTTP error envelope |
| `strict-error-internal` | strict-only HTTP 500 retry projection |
