## Why

Issue #110 (work package 16) requires generated files to reach both Gateway clients. Canonical main already supports input files, reasoning files, metadata and configured fallback, but rejects ordinary generated `file` output.

## What Changes

- Map registered generated-file data and URL variants through strict unary and SSE DTOs, preserving media type, opaque metadata and ordering while omitting provider-local filenames.
- Decode these responses in the independent Go Grafana client, retaining existing resource and cancellation ownership.
- Preserve the registered URL `originalUrl` field with the verified narrow provider-domain prerequisite; prove its exact-pin presence rather than copying historical issue pins.
- Verify aggregate response/frame limits before write or base64 expansion, selected-candidate commitment and independently configured operator/consumer capture.
- Add direct registered TypeScript/Go HTTP consumption, strict schema and frontend assembly evidence, with accurate documentation of synthetic provenance and publication limitations.

## Capabilities

### New Capabilities

- `gateway-generated-media`: Generated-file transport, bounds, lifecycle and cross-client evidence.

### Modified Capabilities

- `grafana-gateway-client`: Add generated-file decoding and remove ordinary files from the unsupported SSE family scenario.

## Impact

AGPL implementation remains in `ai-gateway/providerwire/v4` and its contract workspace. Apache changes remain in `provider` and `providers/grafana`, with no reverse dependency or committed local replacements. Root integration tests can exercise the Apache client through an independently started HTTP server. Dependencies 15 and the native-options, fallback and metadata foundations are present on canonical main; stopped work and #303 are not prerequisites. No image/video/speech model API, URL download service, raw events, routing/BYOK or diagnostics platform is introduced.
