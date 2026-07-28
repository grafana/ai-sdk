## Why

The provider wire was designed Go-to-Go (see `2026-04-30-lossless-provider-wire`
D6/D4 and its Non-Goals). A PoC driving the hosted `aisdkprovider` server with
the upstream Vercel AI SDK (`@ai-sdk/gateway` + `ai`) showed the transport,
headers, framing, stream parts, `finishReason`, and `usage` are already
compatible — but requests fail to decode because upstream serializes a few
shapes differently: `system` message content as a bare string, tool-result
`output` with a single `value`, and file `data` as a tagged union. We want the
hosted endpoint to accept upstream gateway clients without abandoning the
canonical Go-to-Go wire form.

## What Changes

- Make the `provider` package decoders **tolerant** of the upstream
  `LanguageModelV4` JSON shapes, on decode only:
  - `Message`: accept `content` as a JSON **string** (wrapping it into a single
    text `ContentPart`) in addition to the canonical `[]ContentPart` array.
  - `ToolResultOutput`: accept the upstream single-`value` shape
    (`{type,value}`) in addition to the canonical split `text`/`json`/`content`/
    `reason` fields.
  - `DataContent`: accept the upstream tagged file-data union
    (`{type:"data",data}` / `{type:"url",url}`) in addition to the canonical
    `{bytes|base64|url}` fields.
- **Encoders are unchanged.** Go still emits the canonical shapes, so the
  Go↔Go wire bytes are byte-for-byte identical and no conformance fixtures
  need regenerating. This is purely additive inbound tolerance.
- Add regression tests: (a) the canonical Go↔Go round-trip still holds, and
  (b) real upstream-gateway JSON bodies decode into the expected Go values.
- Document the newly-in-scope inbound cross-language tolerance (previously a
  Non-Goal) and its known gaps (mid-stream `error` part and provider-executed
  tool-result/file *responses* remain Go-shaped; not part of the request path).

Not changing: the canonical serialization (D6/D4 stay as the emitted form),
auth, routing, or the orchestration/UI wire.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `provider-wire`: adds a requirement that the wire decoders accept upstream
  `LanguageModelV4` JSON shapes for `system` message content, tool-result
  output, and file data, while the encoders continue to emit the canonical
  Go form (existing round-trip requirements unchanged).

## Impact

- Code: `provider/message.go`, `provider/content.go`, `provider/types.go`
  (add `UnmarshalJSON` methods); tests in `provider/`.
- No change to `provider/wire/`, `providers/grafana/`, or the hosted
  `aisdkprovider` server code — they gain upstream tolerance transitively via
  the shared `provider` decoders.
- No wire-byte change ⇒ no conformance regeneration; Go↔Go behavior identical.
