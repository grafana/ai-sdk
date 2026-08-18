# Bidirectional upstream-client interop

This harness runs a real upstream Vercel AI SDK client (`@ai-sdk/gateway` +
`ai`) against the Go provider-wire server and asserts two-way compatibility. It
is the executable contract for `provider-wire-upstream-full-compat`: it proves a
stock upstream client interoperates with the Go server for streaming text,
required empty delta fields, tool calls, provider-executed tool results, inline
and URL-valued file input/output, and errors (continued streaming after provider
errors and pre-stream failures).

## How it works

- `testserver/` serves mock `provider.LanguageModel` implementations through
  the real public `gateway/providerwire` handler (via a local `replace`). The
  scenario is selected by the model id the gateway forwards in the
  `ai-language-model-id` header.
- `global-setup.ts` builds and boots the Go server on an ephemeral port and
  writes the gateway base URL (`http://127.0.0.1:PORT/api/v1/aisdk`).
- `interop.test.ts` points a real `@ai-sdk/gateway` provider at that base URL
  and drives scenarios through `doStream`, `streamText`, or `generateText`.

## Run

```bash
mise run test-interop
# or, from this directory:
pnpm test
```

## Scenarios

| Scenario (model id)    | Exercises                                              |
| ---------------------- | ------------------------------------------------------ |
| `stream-text`          | streaming text + system prompt (system-as-string req)  |
| `empty-deltas`         | required empty text/reasoning/tool-input delta fields  |
| `tool-call`            | client-executed tool-call round trip                   |
| `provider-tool-result` | provider-executed tool-result (`result` value)         |
| `file-input`           | upstream file-data union decoded server-side           |
| `file-output`          | file stream part emitted with inline `data`             |
| `file-output-url`      | file/reasoning-file parts with URL-valued `data`        |
| `error-mid-stream`     | continued ordered parts after a provider `error`        |
| `error-pre-stream`     | pre-stream HTTP `{"error":{...}}` envelope             |
