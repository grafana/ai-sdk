## Context and registered reference

WP15 / #109 enables file inputs at the Gateway boundary after the independent Apache `apache-file-inputs` producer. The registered reference is `test/conformance/upstream.yaml` commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d` (`@ai-sdk/gateway@4.0.87`, `@ai-sdk/provider@4.0.17`, `ai@7.0.107`); comparisons use that exact source and npm client rather than an unrelated upstream checkout HEAD. The complete ProviderWire schema already defines the file union. Before this change the strict mapper rejects ordinary files even where the registered client can encode them; existing option/header mapping and host policy remain authoritative.

The producer PR (#234) owns selected-arm constructors/validation, presence-aware filenames, UI/model conversion, independent Go-client projection, native provider conversion, and reusable logger/Agent Observability adaptation. This consumer PR (#235) keeps already-merged published module requirements while the explicit Gateway candidate workspace verifies coordinated source. Standalone release builds require published compatible modules. The Gateway remains absent from the root workspace and Apache production imports.

## Decisions

### 1. Decode only the supported file-input branches

After complete request-schema validation, private typed DTOs map ordinary user/assistant `file` parts and `file` entries inside supported tool-result `content` results. The mapper uses public provider constructors, preserving selected data, URL, reference, and text arms even when data/text is empty. Carry order, media type, and nil/empty/non-empty filename presence into provider-domain values. Reuse the same data-arm mapper for nested result entries. Never fetch a submitted URL.

Reject schema-invalid missing/null/inactive members, mixed arms, references with forbidden/non-string members, role violations, and typed null filenames before resolution or model invocation. Valid but deferred reasoning files, generated-output families, custom content, approvals, and provider-executed history remain fixed unsupported errors. The comprehensive registered request golden contains such siblings, so it remains a rejection fixture; focused exact-client goldens prove the newly admitted subset. The mapper does not adopt provider-specific MIME, URL-scheme, or reference policy: native converters in #234 enforce those rules before external provider I/O.

### 2. Keep options at their original scope

Represent supported message-level and ordinary file-entry options as opaque `provider.RawProviderOption`, retaining nested null, false, zero, empty strings and arrays, and explicitly empty namespace objects. Apply the existing protected-field and selected-backend policies to file-entry and function-tool options; reject reserved host namespaces `gateway`, `grafana`, and `grafana-ai-sdk`. Do not promote file options to call-level options or headers. Existing call-level options, body headers, and text-part options retain their mapping; output-level and non-file nested result options remain deferred.

### 3. Preserve text fallback, not file fallback

Text-only fallback remains eligible when ordinary message-option namespaces retained by the selected-backend policy are empty JSON objects. The route guard checks them without stripping or mutating the request it receives: every candidate sees the same retained value. A retained namespace containing any member (even a null/false/empty one), file part, or effectful tool history remains ineligible before physical candidate invocation. Invalid and reserved namespaces fail earlier in mapping; irrelevant namespaces are removed by the backend policy. This does not enable replay or file fallback.

### 4. Bound work and protect observations

Keep the existing overflow-safe complete-request byte budget over JSON text, escaping, and base64 expansion; no arbitrary per-file quota is introduced. Schema-invalid and unsupported requests fail before execution, while cancellation propagates on admitted unary and streaming calls. Gateway logs, metrics and Agent Observability remain a single canonical, metadata-only logical observation and exclude file data, inline text, URLs/query strings, references, filenames, options, credentials, and backend identity. File URLs are forwarded as references, never retrieved by the Gateway. Existing safe response/SSE families remain unchanged.

### 5. Require independent, provenance-valid evidence

Record focused requests with the exact pinned TypeScript Gateway client and replay them through the production Go mapper in both modes, asserting arms, media types, filename presence, scopes, and invocation counts. Compare the independent Go client's semantic body for the same cases. Authenticated real-command tests route both clients through the candidate-source Anthropic module for user files and nested tool-result continuation, including selected inline-text and URL/PDF files; fake providers make request effects deterministic without claiming live-provider acceptance. Negative tests assert zero execution-boundary, resolver, and model calls; byte-boundary, no-URL-fetch, cancellation and hostile-marker tests cover safety/privacy. Do not invent or modify provider `recorded/` or `upstream/` fixture inputs.

## Dependency and rollout

Keep internal `ai-gateway/go.mod` requirements on already-merged revisions without committed `replace` directives. Validate the coordinated changes through `go.gateway.work` and the merged-pin/source gates; reserve `GOWORK=off` standalone validation for release readiness. Run providerwire, authenticated-command, integration, parity, build, test, vet, lint, format and license-boundary checks. Preserve the approved corresponding-source offer when deploying an image. Reverting to the previous coherent Gateway set restores safe unsupported-file behavior; there is no storage migration. Live-provider and deployed-ingress acceptance remain separate operational evidence.
