## 1. Implement tolerant decoders (encoders unchanged)

- [x] 1.1 Add `Message.UnmarshalJSON` in `provider/message.go`: decode
  `role`/`content`/`providerOptions`; if `content` is a JSON string, wrap into a
  single `text` `ContentPart`; otherwise decode `content` as `[]ContentPart`.
- [x] 1.2 Add `ToolResultOutput.UnmarshalJSON` in `provider/types.go`: decode
  canonical split fields; when a `value` field is present, map it by `type`
  (`text`/`error-text`→`Text`, `json`/`error-json`→`JSON`, `content`→`Content`).
- [x] 1.3 Add `DataContent.UnmarshalJSON` in `provider/content.go`: if the object
  carries a `type` field, decode the upstream union (`data`→`Base64`,
  `url`→`URL`); otherwise decode the canonical `{bytes|base64|url}` fields.
- [x] 1.4 Confirm no `MarshalJSON` is added for these types (canonical emitted
  form unchanged).

## 2. Tests

- [x] 2.1 Keep `TestCallOptions_WireRoundTrip` green (canonical round-trip with
  system + split tool-result + `DataContent{URL}`).
- [x] 2.2 Add decode tests for each upstream shape (system string, tool-result
  `value` for text/json, file `data` union url + base64).
- [x] 2.3 Add a wire-level test that decodes a real upstream `@ai-sdk/gateway`
  request body (system + user) via `wire.DecodeCallOptions`.
- [x] 2.4 Add an assertion that canonical emitted JSON is unchanged
  (system `content` is an array, tool-result uses split fields, data uses
  `{"url":...}`).

## 3. Verify

- [x] 3.1 `go test ./...` in root module and `provider/wire`.
- [x] 3.2 `go test ./...` in `providers/anthropic`, `providers/grafana`.
- [x] 3.3 Conformance: `go test -tags conformance ./test/conformance/grafana/...`
  (and anthropic) — expect pass with no fixture changes.
- [x] 3.4 `gofmt`/`go vet` clean on changed files.
- [x] 3.5 Re-run `poc/gateway-interop` **without** `POC_COMPAT` and confirm the
  upstream gateway system-prompt scenarios now pass end-to-end.
