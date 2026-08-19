# Bidirectional upstream-client interop

This harness runs the exact registered Vercel AI SDK packages against Go
provider-wire handlers. It keeps two evidence lanes separate.

## Legacy transport lane

The default suite drives a real `@ai-sdk/gateway` client against the public
`gateway/providerwire` handler. It proves the established unary and streaming
transport for text, required empty deltas, tools, file data, and errors.

`testserver/` selects a mock `provider.LanguageModel` by the forwarded
`ai-language-model-id` header. `global-setup.ts` builds the server on an
ephemeral loopback port, and `interop.test.ts` uses the legacy
`/api/v1/aisdk` mount without selecting V4.

Run it with:

```bash
mise run test-interop
```

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

## Strict V4 evidence lane

The ProviderWire V4 suite combines pinned request captures and curated response
consumption with live HTTP calls to the real strict `gateway/providerwire/v4`
handler. `providerwire-v4/runtime.test.ts` covers direct `doGenerate`,
`ai.generateText`, explicit empty and opaque request values, structured response
format adaptation, response privacy, safe 429/5xx errors, and pre-resolution
rejection of unsupported headers, Gateway controls, raw intent, and streaming.

Run the exact-package non-mutating suite with:

```bash
mise run test-interop-contract
```

Run every V4 contract, runtime, independent unary oracle, and selected Bedrock
handler check with:

```bash
mise run check-providerwire-v4
```

Generated capture replacement is deliberately separate:

```bash
mise run update-providerwire-v4-artifacts
```

The V4 evidence does not claim a streaming V4 service, Go client, Grafana V4
adoption, frontend runtime, or Vercel private-server behavior. Authorities,
directions, artifact commands, claims, and bounded non-claims are recorded in
[`providerwire-v4/INDEX.yaml`](providerwire-v4/INDEX.yaml).
