## Context

Two prior decisions in `2026-04-30-lossless-provider-wire` made the provider
wire Go-to-Go only: **D6** (system content serialized as `[]ContentPart`) and
**D4** (error stream part serialized with an `apiCallError` field). The
follow-up `provider-wire-upstream-decode-compat` made the **decoders** tolerant
of upstream `LanguageModelV4` JSON, which unblocks the request path (an upstream
`@ai-sdk/gateway` client can send text + system prompts + text/JSON tool
results). This change closes the remaining gap: make the wire **symmetric** so
the response path and full round-trips work with an upstream client, superseding
D6/D4.

A PoC (`poc/gateway-interop`) established the transport, headers, framing,
`finishReason` (`{unified,raw}` — matches), and `usage` (nested — matches) are
already compatible. The remaining divergences are enumerated below.

## Goals / Non-Goals

**Goals:**

- An upstream `@ai-sdk/gateway` client works end-to-end against the hosted
  server: streaming text, tool calls, provider-executed tools, files, and
  errors — not just the text happy path.
- Encoders emit upstream `LanguageModelV4` JSON for every divergent type;
  decoders accept both upstream and legacy Go encodings (smooth rollout).
- A bidirectional conformance harness locks two-way behavior.

**Non-Goals:**

- Changing the transport, routing, headers, or auth (already compatible).
- Changing `FinishReason`/`Usage`/`response-metadata`/text parts (already
  match upstream).
- Supporting gateway endpoints beyond `/language-model` (metadata, credits,
  spend, generation info).

## Divergence inventory

Directions: **Req** = upstream client → Go server; **Resp** = Go server →
upstream client. "Decode today" = already handled by
`provider-wire-upstream-decode-compat`.

Decisive fact (verified): the gateway performs **no schema validation** — it uses
`z.any()` for success and error handlers (`gateway-language-model.ts:92-96,
135-139`) and passes parts through. So divergences do not fail at the gateway;
they fail (TypeError) or silently corrupt **downstream in `ai` core**, which
dereferences specific fields. That is why file/data mismatches are hard failures
(`part.data.url.toString()` on `undefined`) while field-name mismatches are
silent-loss (resolve to `undefined`).

**Request direction** — Go only decodes (it is the server). Fully covered by the
decode-compat change + pre-existing matches, except two rare edge cases:

| # | Shape | Severity | Note |
|---|---|---|---|
| Q4 | file-data `reference` / inline `text` variants | silent-loss (rare) | decode drops to empty `DataContent` (`content.go`) |
| Q5 | tool-result `content` with a nested `file.data` union | silent-loss (rare) | `ToolResultContentValue.Data` is a plain string; nested union unmarshal error is swallowed (`types.go`) |

**Response direction** — Go emits; needs symmetric encode changes:

| # | Shape | Upstream JSON (needed by `ai` core) | Go JSON (file) | Severity |
|---|---|---|---|---|
| R1 | `file`/`reasoning-file` **stream** part `data` | `{type:"file",mediaType,data:{type:"data",data:<b64>}}` | `{type:"file",fileData:<b64>,mediaType}` (`stream_part.go:68`) | **HARD-FAIL** (`chunk.data.type` on undefined → `controller.error` kills stream) |
| R2 | `file`/`reasoning-file` **unary** content `data` | `{type:"file",...,data:{type:"data",data:<b64>}}` | `{type:"file",data:{base64:<b64>},mediaType}` (`types.go:153`, `content.go:10-14`) | **HARD-FAIL** (`part.data.url.toString()` on undefined for base64/bytes) |
| R4 | **unary** `tool-call` `input` | `input` is a **string** (`language-model-v4-tool-call.ts:23`) | object (`GenerateContentPart.Input json.RawMessage`, `types.go:145`) | **HARD-FAIL** (`parse-tool-call.ts:191` `input.trim()` throws → tool-call `invalid`) — unary tool calling broken. (Stream tool-call already emits a string ✅) |
| R3 | **stream** `tool-result` payload | `{type:"tool-result",...,result:<JSONValue>,isError?,dynamic?}` | `{...,output:{type,json/text}}` (`stream_part.go:59`) | silent-loss (provider-executed tool output dropped) |
| R5 | **stream** `source` part | flat `{type:"source",sourceType,id,url,title?,...}` | nested `{type:"source",source:{...}}` (`stream_part.go:67`) | silent-loss (source dropped) |
| R6 | `error` stream part | `{type:"error",error:<...>}` | `{type:"error",apiCallError:{...}}` (`stream_part.go:88`) | error detail lost (generic "An error occurred.") |
| R8 | HTTP error envelope (unary/pre-stream) | `{error:{message,type?,code?,param?},generationId?}` (`create-gateway-error.ts:130-142`, zod-validated) | flat `APICallError` (`api_call_error.go:18-32`, `wire/errors.go`) | typed errors lost → generic `GatewayResponseError`; Go `isRetryable` ignored |
| R7 | **unary** `source` `title` | `title?` (url) / required (document) | no `Title` field on `GenerateContentPart` (`types.go:137-157`) | silent-loss (title dropped) |

Confirmed already-matching (no change): `finishReason {unified,raw}`, `usage`
(nested), `response-metadata`, `text/reasoning/tool-input-*`, **stream**
`tool-call` (input string ✅), **unary** `tool-result` (`result`+`isError` ✅),
`toolChoice`, `finish`, `raw`, `custom`, function/provider tools, and all scalar
`CallOptions` fields. (Inventory verified against `ai@7.0.11` /
`@ai-sdk/gateway@4.0.7`.)

**Empirical corroboration** (advanced PoC, `poc/gateway-interop-advanced/`, real
`provider`+`wire` + upstream client):

- **R6** (error stream part) — confirmed BREAK: server sent
  `{"type":"error","apiCallError":{"message":"boom",...}}`; client saw
  `error: null`, `onError = {}` (message/status/retryable lost).
- **R3** (provider-executed **stream** `tool-result`) — confirmed BREAK: server
  sent `output:{type:"json",json:{...}}`; the client's tool-result part arrived
  with **no `result`** (value dropped).
- Request-path PASSes (decode tolerance working): client-executed tool-call
  round-trip (2nd request's `output:{type:"json",value}` decoded), and
  file/image input (`data:{type:"data",data:<b64>}` decoded).
- Cosmetic (tolerated): stream parts emit a zero `timestamp`
  (`"0001-01-01T00:00:00Z"`) on parts that shouldn't carry one; the upstream
  client ignores it, but consider `omitempty`/zero-time suppression when
  touching `StreamPart.MarshalJSON`.

R1/R2 (response file emit) and R4 (unary tool-call `input` string) were not
exercised by the PoC; they are covered by the type-level inventory above.

## Decisions

### D1. Add `MarshalJSON` alongside the tolerant `UnmarshalJSON`

For each divergent type emit the upstream shape while decoding both. Priority
order: R1/R2/R4 (hard-fail) first, then R3/R5/R6/R8 (silent-loss), R7 last.

- `DataContent.MarshalJSON`: emit the tagged union (`bytes`/`base64` →
  `{type:"data",data:<base64>}`, `url` → `{type:"url",url}`); add `reference` /
  inline `text` carriers (fixes **R1** stream file + **R2** unary file, and Q4).
- `StreamPart.MarshalJSON` (extend the existing type-aware marshaler): per
  `Type` — `file`/`reasoning-file` → `data` union + `mediaType`, drop `fileData`
  (**R1**); `tool-result` → `result` (raw JSON value) + `isError` + `dynamic`,
  drop `output` (**R3**); `source` → flat `sourceType`/`id`/`url`/`title`/…
  inline, not nested (**R5**); `error` → `{type:"error",error:<APICallError>}`
  (**R6**). Decoders accept both old and new forms.
- `GenerateContentPart` (unary): emit `tool-call` `input` as a **stringified
  JSON string** (**R4**, the one that breaks unary tool calling); add a `Title`
  field for `source` (**R7**); `file` `data` marshals as the union via the
  `DataContent` fix (**R2**). Note: unary `tool-result` already emits
  `result`+`isError` — leave it.
- Error envelope (`wire.WriteErrorResponse` / `EncodeAPICallError`): emit
  `{error:{<APICallError fields>}}` so the gateway's
  `createGatewayErrorFromResponse` (zod-validated: `error.message` required,
  `error.type` nullish) surfaces the real message. `DecodeErrorResponse` accepts
  both wrapped and legacy flat forms (**R8**).
  - **Refinement (implementation):** a synthesized status→`type` is intentionally
    NOT injected. The spec scenario body shows no `type`; the upstream gateway
    already surfaces the message with a missing/nullish `type` (falling to
    `internal_server_error`); and injecting a status-derived `type` into the body
    would pollute the response body that the Go client (`providers/grafana`)
    parses for *provider-structured* error categorization, reclassifying
    otherwise-uncategorized errors. Keeping the envelope a pure wrapper preserves
    Go-client semantics while satisfying the message-surfacing requirement.
- `Message.MarshalJSON` (system → string) and `ToolResultOutput` request/prompt
  `{type,value}`: only needed for Go→gateway *request* byte-parity; not required
  for the TS-gateway→Go server path (the server never sends requests to the
  gateway). Include for completeness/symmetry, low priority.

### D2. Supersede D6 and D4

This change explicitly overrides `2026-04-30-lossless-provider-wire` D6 (system
as array) and D4 (error as `apiCallError`). Record the supersession in that
change's notes and in `test/conformance/PARITY.md`.

### D3. Rollout via decode tolerance

Because decoders accept both shapes, a mixed-version fleet (old Go client + new
server, or vice versa) keeps working during rollout. The Go client
(`providers/grafana`) and hosted server move to the new emitted shapes together;
no flag day required.

### D4. Bidirectional conformance

Add a harness that runs a real upstream `@ai-sdk/gateway` client against the Go
server for: streaming text, tool-call round-trip, provider-executed tool result,
file input+output, and mid-stream + pre-stream errors. Base it on
`poc/gateway-interop` and `poc/gateway-interop-advanced`. This becomes the
executable contract for two-way parity.

## Risks / Trade-offs

- **Go↔Go wire bytes change** for the affected types, BUT **no conformance
  fixture regeneration is needed** (verified): `test/conformance/grafana`
  asserts the downstream **Anthropic `/v1/messages`** requests and the final
  **UIMessageChunks**, not the intermediate provider-wire JSON; since encode and
  decode change together (same Go code on both hops), the decoded structs and
  all derived artifacts are invariant. Only **hand-written unit tests that
  assert wire bytes** must be updated: `provider/stream_part_test.go` (file
  `fileData`→`data`, error `apiCallError`→`error`, tool-result, source),
  `provider/types_test.go`, `provider/content_test.go`,
  `provider/api_call_error_test.go`, `provider/wire/{sse,errors}_test.go`,
  `provider/call_options_wire_test.go`, `provider/upstream_decode_compat_test.go`.
- **`DataContent` gains fields** (`reference`, inline `text`) → additive; guard
  `Validate()` for the new variants.
- **Parity governance** → run `mise run parity-check`; update `upstream.yaml` /
  `PARITY.md`; note this removes the "documented deviation" status of D6/D4.
- **Two emitted-shape migrations in the codebase** (decode-compat already
  landed; this adds encode) → keep both changes' tests to prevent regressions.

## Migration Plan

1. Reconcile the divergence table with the research subagent's line-cited
   inventory.
2. Add `MarshalJSON` for the divergent types; keep `UnmarshalJSON` tolerant.
3. Update `wire.WriteErrorResponse`/`EncodeAPICallError` to the `{error:{...}}`
   envelope (decode both).
4. Regenerate/review conformance fixtures; run full test + conformance +
   parity-check.
5. Add the bidirectional upstream-client conformance harness.
6. Update `docs/` + `aisdkprovider` README (remove limitations caveats); record
   D6/D4 supersession.

Rollback: revert; decode tolerance means older peers still interoperate.
