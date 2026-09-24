# Sources

Unary and streaming responses support registered URL and document sources.
Document titles are always present, including an empty string. Empty optional
URL titles and document filenames are omitted. The Go client exposes unary
titles in both Title and the legacy Text field.

The Gateway replaces native source identifiers with response-local IDs. Repeated
references to the same source type and native ID retain one public ID. IDs are
not persistent identifiers across requests.

Only bounded numeric citation positions are public metadata: OpenAI index and
Anthropic start/end page or character positions under the citation namespace.
Unknown fields, native file/container IDs, encrypted indexes and cited text are
omitted. Malformed positions are omitted; oversized recognized metadata fails
safely. OpenAI file_path sources use title Document and omit filename so native
file IDs used as display text do not leak. Other titles, filenames and URLs are
application content; this policy does not redact arbitrary document content.

Sources may appear between text/tool events without closing their blocks.
Sources commit a fallback candidate; a subsequent failure cannot replay the
generation on another backend. Source output does not enable provider tools,
reasoning, generated files or raw output.

Deterministic command and client tests establish mapping, framing and privacy,
not live provider acceptance. The Gateway pins the published logger and Agent
Observability revisions that count source-only responses as first output while
continuing to omit source content from capture.

See [text observability](text-observability.md) for telemetry privacy.
