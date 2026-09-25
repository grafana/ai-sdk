## Why

[Issue #109](https://github.com/grafana/ai-sdk/issues/109) / WP15 requires selected file inputs to cross the strict AI Gateway service in both execution modes. The Apache producer change `apache-file-inputs` provides the public file-data, filename, native-provider, and independent Go-client contract; without Gateway mapping, authenticated file-bearing requests still fail as unsupported.

## What Changes

- Accept user/assistant ordinary file parts and file entries in supported tool-result history, preserving all four selected data arms, media types, order, filename presence, and scoped opaque provider options.
- Reject malformed or mixed file arms, forbidden references, reserved namespaces, and still-deferred capabilities before model invocation. Bound complete encoded requests, preserve cancellation, and never fetch file URLs in the Gateway.
- Retain text-only fallback eligibility for empty ordinary message-option namespaces without allowing files, active options, or effectful history through fallback.
- Keep file payloads, URLs, references, filenames, and options out of Gateway logical logs, metrics, Agent Observability, and safe errors. Verify Vercel and Go clients through authenticated unary/streaming calls and native request capture.
- Pin the published Apache prerequisites—including corrected Anthropic `206427960ce2`—with readonly standalone module validation. Keep generated output (WP16), reasoning-file runtime (WP17), custom content, approvals/provider-executed history, top-level provider options, body headers, and file fallback deferred.

## Capabilities

### New Capabilities

- `gateway-file-inputs`: strict bounded mapping, scoped options, privacy, and cross-client runtime evidence.

### Modified Capabilities

- `gateway-ordered-text-fallback`: retain empty ordinary message-option namespaces without enabling file fallback.
- `gateway-unary-function-tools`: admit supported file-result content in existing tool history.
- `providerwire-v4-unary-runtime`: replace blanket file/message-option rejection with the supported subset shared by unary and streaming execution.

## Impact

The Gateway module changes private ProviderWire DTO mapping, fallback guard, host command/privacy tests, module pins, and Gateway-owned operator docs. Producer-side SDK/provider/client/UI changes and their five OpenSpec deltas belong exclusively to the stacked Apache prerequisite PR (#234). This consumer PR (#235) targets that branch; it is not independently mergeable before those immutable modules are available. Authority remains `test/conformance/upstream.yaml` at commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d`; no upstream baseline upgrade is included.
