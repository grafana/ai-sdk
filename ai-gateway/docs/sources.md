# Sources

Use sources to show readers which web pages or documents a model referenced in
its answer. The Gateway returns sources when the selected model provides them;
not every model or request produces citations.

## Display citations

A web source points to a URL. Show its title when available, or use the URL as
its label. A document source identifies a file or document rather than a web
page. Its title or filename may be a provider-assigned identifier, so choose a
readable fallback label when neither is useful. A document reference does not
provide a download link.

Keep source IDs as reference values, not display labels or permanent document
keys. IDs can be empty or repeated, including across web and document sources.
If your citation list needs unique UI keys, assign those separately rather than
assuming every source ID is unique.

Treat source URLs and labels as untrusted content. Escape labels when rendering
and validate links before making them clickable; see the
[security guide](../../docs/best-practices/security.md).

## Include sources in a chat UI

When your application forwards a response to a chat frontend, enable source
forwarding and render the source parts alongside the answer. See
[Streaming over HTTP](../../docs/guides/streaming-http.md#control-client-visible-content)
for a Go server example. Returning sources from the Gateway alone does not make
them visible in your UI.

## Understand the limits

Some sources include page or character positions that can help readers locate
the reference. The Gateway currently returns limited citation metadata, not all
provider annotations or cited passages. Design your citation UI to work without
those extra details.

A citation is a model-provided reference, not verification that the answer is
correct or that the linked content is still available. The Gateway does not
retrieve source URLs for you.

Sources returned to your application are not automatically captured in Gateway
logs or telemetry. See [text observability](text-observability.md#returned-values-and-consumer-observation)
for the distinction and the [Go client guide](../../docs/providers/grafana-gateway.md)
for accessing response data.

---

← [Model catalog](model-catalog.md) · [Docs index](../../docs/README.md) · [Text observability →](text-observability.md)
