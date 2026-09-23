# Operate file inputs

Direct routes accept user/assistant file inputs and supported tool-result file
content in both unary and streaming calls. The strict ProviderWire adapter maps
inline data, URLs, provider references, and inline text, including selected
empty data/text and explicitly empty filenames. Message-level and file-entry
provider options remain scoped to their original part; reserved Gateway
namespaces are rejected before provider invocation.

## Enable a direct backend deliberately

Backend conversions retain their own media, URL-scheme, provider-reference,
filename-default, and warning/error rules. Schema acceptance is not proof that
any particular backend accepts the input. The Gateway never downloads file
URLs. Text-only fallback routes reject files and effectful tool history before
a candidate executes; an empty message-option namespace remains eligible for
text fallback without enabling file fallback.

Bound the complete JSON request, including base64 expansion and JSON escaping,
using the service request-byte limit. Verify an authenticated file-input unary
and stream call against the selected backend and both clients before enabling
the capability internally. The deterministic command smoke covers Anthropic
file inputs and file-result continuation without claiming a live provider
recording. Generated media output and reasoning files have separate capability
boundaries.

## Keep content out of operations data

Logical logs, metrics, and Agent Observability are metadata-only: do not add
file data, inline text, URL/query strings, provider references, filenames,
provider options, or backend identity to public telemetry or metric labels.
Keep native provider request payloads out of operational logging. Deploy the
image with notices and the approved corresponding-source offer for its exact
published dependencies and source revision.
