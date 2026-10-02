# Sources

Unary and streaming responses preserve registered URL and document sources.
Document titles and source IDs are always present, including empty strings.
Empty optional URL titles and document filenames are omitted because Go cannot
distinguish absent and empty optional strings. The Go client exposes unary
source titles in both Title and the legacy Text field. A nonempty Title wins
when the native unary provider supplies both; otherwise Text supplies the title.

Native source IDs survive without sequential replacement, deduplication or
cross-variant collision repair. Repeated references and URL/document sources
with equal IDs retain their values and order. Their lifetime is provider-owned,
not a Gateway persistence guarantee. Source IDs are not public route IDs; their
resource bound is the containing unary response or complete SSE frame, not the
removed 1024-byte identity-map cap.

OpenAI/Azure `file_path` sources retain the adapter-generated source ID and
native title/filename, including file IDs used as display content. Consumers
must stop relying on `source-N` IDs, `Document` substitution or removed filenames.
The Gateway neither fetches nor canonicalizes source URLs.

The existing bounded numeric citation projection remains: OpenAI/Azure index
and Anthropic start/end page or character positions under `citation`.
Unknown namespaces/fields and cited text are still omitted. This is an
outstanding opaque-metadata implementation gap, not an approved privacy policy
or a claim of complete native parity. This scalar/display change does not
expand that codec. Malformed positions are omitted; oversized recognized
metadata fails safely.

Sources may appear between text/tool events without closing their blocks.
The first provider part commits a fallback candidate; a subsequent failure
cannot replay the generation on another backend. Source output does not enable
additional provider tools or raw output.

Deterministic raw/schema, both-client and authenticated command tests establish
mapping, framing and bounds, not live provider acceptance. Schema-parsed UI
assembly preserves repeated native source IDs/display; response identity in
that frontend scenario is explicitly mapped by a test-only message-metadata
callback, not an automatic UI field. Warnings remain provider/client values.

Operator capture remains metadata-only and canonical without altering returned
source values. Consumer middleware is independently configured; see
[text observability](text-observability.md) and the
[Go client guide](../../docs/providers/grafana-gateway.md#native-response-values).
The Gateway image uses same-revision source through `go.gateway.work` and
requires its build gate. Published middleware revisions still need later
module releases to adopt source-only first-output timing independently.
