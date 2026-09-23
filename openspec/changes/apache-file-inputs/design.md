## Context

This is the independently mergeable Apache producer portion of WP15 / #109. The registered reference is `test/conformance/upstream.yaml` at `08ae5ad05bc12496dd1ffcf64e34419e0831300d`: `ai@7.0.107`, `@ai-sdk/provider@4.0.17`, Anthropic 4.0.58, Bedrock 5.0.88, OpenAI 4.0.71, OpenAI-compatible 3.0.53, and Gateway 4.0.87. The matching TypeScript file-data, UI conversion, Gateway client projection, and provider request converters—not upstream HEAD—define the observable semantics.

The existing Go `DataContent` cannot publicly select empty text or URL values; ordinary string filenames lose absent versus explicit empty; consumers often branch on non-empty payloads. The separate Gateway runtime requires published producer modules rather than workspace-only resolution.

## Decisions

### Represent selection without a new union hierarchy

Keep the flat `DataContent` with private typed selection and add public constructors/inspectors for all four tagged arms. Bytes and base64 are alternate forms of the same data arm. One resolver validates construction, tagged/legacy decoding, and marshaling; tagged decoding rejects even inactive empty/null members before they can disappear. References require an object with string-valued provider IDs and no reserved `type`. Structurally valid references are resolved by the native provider, not by the provider-independent domain.

### Preserve filename presence only for request inputs

Use `*string` for `provider.ContentPart.Filename`, nested `ToolResultContentValue.Filename`, and ordinary UI `FilePart.Filename`. Nil means absent; pointer-to-empty means present empty. Preserve presence through UI JSON, model conversion, Go-client projection, and native request conversion. Keep source information, generated content, UI source documents, and file SSE chunks on their existing descriptive-string semantics. Native defaults remain provider-specific: upstream OpenAI/OpenAI-compatible use nullish defaults; Bedrock retains empty-name normalization.

### Delegate native support to each provider

Validate direct-call file selection before outbound I/O, then use native request converters for media, URL, reference, role, and filename rules. Anthropic text data is a text document regardless of the declared media type; URL image/document and inline PDF tool-result files remain content rather than empty results. Inline PDF tool output adds the PDF beta, URL-only results do not. Bedrock retains its documented warning/omission for tool-result URLs. No cross-provider MIME allowlist or Gateway URL fetching is introduced.

### Keep the Apache boundary independently consumable

The Go Gateway client projects selected arms and opaque scoped options without importing Gateway DTOs; reusable observation honors payload-capture configuration and metadata-only redaction. Publish immutable root, adapter, Bedrock, and corrected Anthropic revisions in dependency order. The separate Gateway PR pins proxy-resolvable versions and verifies `GOWORK=off`; no committed production `replace` or Gateway import enters Apache modules.

## Evidence and limits

Unit/native-request tests cover empty arms, conflicts, invalid references, absent/empty native filename defaults, Anthropic text/PDF/URL cases, provider-specific warnings, and capture/redaction. A deterministic Go testserver and pinned TypeScript UI conversion compare filename presence through model/client projection. The exact registered Gateway client produces request goldens; the shape witness updates only for reviewed provider contract fields. Existing provenance-valid conformance fixtures are replayed unchanged. Synthetic native requests and deterministic UI tests do not establish live-provider or deployed ingress behavior.

Generated media output (WP16), reasoning-file runtime (WP17), unrelated tools, and file fallback are outside this producer change. The stacked `gateway-file-inputs` change owns strict Gateway mapping, host bounds/privacy, and authenticated runtime evidence.
