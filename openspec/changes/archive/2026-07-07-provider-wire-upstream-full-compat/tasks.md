## 1. Reconcile the divergence inventory

- [x] 1.1 Fold the research subagent's line-cited inventory into `design.md`'s
  table (confirm source stream discriminator, reasoning-file, custom `kind`,
  tool-call `input` encoding). (Inventory already reconciled in `design.md`;
  verified against `ai@7.0.14` / `@ai-sdk/gateway@4.0.7` upstream sources.)
- [x] 1.2 Confirm downstream provider request snapshots (Anthropic/Bedrock/
  OpenAI) do not assert provider-wire JSON bytes (so only Grafana conformance
  regenerates). (`mise run parity-check` passes with `# fail 0` and no fixture
  regeneration.)

## 2. Encoders (add MarshalJSON; keep tolerant UnmarshalJSON) — hard-fail first

- [x] 2.1 `DataContent.MarshalJSON`: emit tagged union (`data`/`url`); add
  `reference`/inline-`text` carriers. Fixes **R1** (stream file) + **R2** (unary
  file), both hard-fail.
- [x] 2.2 `GenerateContentPart` unary `tool-call` `input` → **stringified JSON
  string** (**R4**, hard-fail: breaks unary tool calling).
- [x] 2.3 `StreamPart.MarshalJSON`: `file`/`reasoning-file` → `data` union +
  `mediaType` (drop `fileData`, **R1**); `tool-result` → `result`+`isError`
  (drop `output`, **R3**); `source` → flat inline (**R5**); `error` →
  `{type:"error",error:{...}}` (**R6**). (Also suppresses zero-value timestamps.)
- [x] 2.4 `GenerateContentPart`: add `Title` field for source (**R7**).
- [x] 2.5 (Low priority / symmetry) `Message.MarshalJSON` system→string and
  request-side `ToolResultOutput` `{type,value}`.

## 3. Error envelope (R8)

- [x] 3.1 `wire.WriteErrorResponse` / `EncodeAPICallError` emit
  `{"error":{<APICallError fields>}}`. A synthesized `type` is intentionally NOT
  injected: the spec scenario shows no `type`, the upstream gateway surfaces the
  message with a missing/nullish type, and injecting one would pollute the
  response body that the Go client parses for provider-structured error
  categorization.
- [x] 3.2 `wire.DecodeErrorResponse` / `DecodeAPICallError` accept both the
  wrapped envelope and the legacy flat form.

## 4. Tests + conformance

- [x] 4.1 Update hand-written wire-byte unit tests (no conformance
  regeneration): `provider/stream_part_test.go`, `provider/content_test.go`,
  `provider/wire/errors_test.go`, `provider/upstream_decode_compat_test.go`
  (renamed `TestUpstreamEmittedForm`). Grafana/anthropic conformance passes
  unchanged.
- [x] 4.2 Add MarshalJSON round-trip tests (encode == upstream JSON; decode both
  shapes): `provider/upstream_encode_compat_test.go` +
  `provider/wire/errors_test.go`.
- [x] 4.3 Add the bidirectional upstream-client conformance harness under
  `test/interop` (built on `poc/gateway-interop-advanced`): streaming text,
  tool-call round-trip, provider-executed tool result, file in/out, mid-stream +
  pre-stream errors. All 7 scenarios pass against a real `@ai-sdk/gateway` client.

## 5. Governance + docs

- [x] 5.1 `mise run parity-check` (passes, `# fail 0`); updated
  `test/conformance/PARITY.md`; recorded supersession of D6/D4 in the archived
  `2026-04-30-lossless-provider-wire/design.md`.
- [x] 5.2 Update `docs/providers/grafana-cloud.md` and the `aisdkprovider`
  README (replaced "known limitations" with full-compatibility statement).
- [x] 5.3 `go test ./...` (root + provider modules), `gofmt`, `go vet` clean.
