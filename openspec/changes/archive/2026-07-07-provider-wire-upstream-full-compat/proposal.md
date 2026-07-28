## Why

`provider-wire-upstream-decode-compat` made the provider-wire **decoders**
tolerant of upstream Vercel AI SDK (`LanguageModelV4`) JSON, so an upstream
`@ai-sdk/gateway` client can send requests to the hosted endpoint. But the
**encoders** still emit the canonical Go-to-Go shapes, so the *response* path
and a few request round-trips remain incompatible: mid-stream errors, provider-
executed tool results, multimodal files, and the exact tool-result/system/file
encodings differ from what the upstream client expects. This change makes the
provider wire **symmetrically** compatible so an upstream TypeScript client
works end-to-end (streaming, tools, files, errors), not just for the text happy
path.

## What Changes

- Make the `provider` types **emit** upstream `LanguageModelV4` JSON (add
  `MarshalJSON` where shapes diverge), while keeping decode tolerant of both the
  upstream and legacy Go shapes:
  - `Message`: system content emitted as a **string** (upstream shape).
  - `ToolResultOutput`: emitted as the single-`value` union (`{type,value}`),
    decoded from both.
  - `DataContent` / file parts: emitted as the tagged union
    (`{type:"data"|"url"|...}`).
  - Error stream part: emitted as `{type:"error", error:{...}}` (upstream),
    carrying the `APICallError` payload under `error`.
  - Any remaining response-path parts (provider-executed tool-call/tool-result,
    source, reasoning-file) reconciled to upstream field names/shapes.
- **BREAKING (Go↔Go wire bytes)**: the canonical emitted JSON changes for the
  affected types. It supersedes decisions **D6** (system as `[]ContentPart`) and
  **D4** (error part as `apiCallError`) from `2026-04-30-lossless-provider-wire`.
  Verified: **no conformance-fixture regeneration is required** (the grafana/
  anthropic fixtures assert downstream Anthropic requests and final
  UIMessageChunks, not the provider-wire JSON); only hand-written provider/wire
  unit tests that assert wire bytes change.
- Add a bidirectional conformance harness that replays a real upstream
  `@ai-sdk/gateway` client against the Go server (build on `poc/gateway-interop`
  and `poc/gateway-interop-advanced`) so two-way compatibility is regression-
  tested.
- Update `docs/` and the `aisdkprovider` server README to state full
  compatibility and remove the "known limitations" caveats.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `provider-wire`: the wire encoders SHALL emit upstream `LanguageModelV4` JSON
  for the divergent types (system message content, tool-result output, file
  data, error stream part, provider-executed tool parts), and decoders SHALL
  accept both upstream and legacy Go encodings. Supersedes the Go-to-Go-only
  emitted shapes from `2026-04-30-lossless-provider-wire` (D6, D4).
- `provider-v4-core-types`: `FinishReason`/`Usage` already match upstream and are
  unaffected; document that the remaining core types now serialize
  upstream-compatibly.

## Impact

- Code: `provider/message.go`, `provider/content.go`, `provider/types.go`,
  `provider/stream_part.go`, `provider/api_call_error.go` (add `MarshalJSON`;
  keep tolerant `UnmarshalJSON`).
- Conformance: **unaffected** (fixtures assert Anthropic requests + final
  UIMessageChunks, not provider-wire bytes). Update hand-written unit tests in
  `provider/` and `provider/wire/` that assert wire bytes.
- Consumers: the Go client (`providers/grafana`) and the hosted `aisdkprovider`
  server both move to the new emitted shapes together; decode tolerance keeps a
  mixed-version window working during rollout.
- Governance: parity-sensitive; run `mise run parity-check` and update
  `test/conformance/PARITY.md` / `upstream.yaml` notes.
