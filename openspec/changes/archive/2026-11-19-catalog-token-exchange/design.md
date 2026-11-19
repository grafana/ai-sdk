## Context

`Provider.request` already serves all three operations. It resolves the call's namespaces, exchanges the CAP credential (optionally with the user's token as the subject), and appends the route to the configured prefix. The Gateway already registers catalog routes and enforces their access checks. Its catalog handlers consume the same ProviderWire protocol as their namespace counterparts.

## Decisions

### Select a catalog in the call context

Export `WithCatalog(ctx context.Context, name string) context.Context` next to `WithNamespace`. The latest selection in a derived context wins; an explicit empty name is invalid, not a reset. Apply the option when a request is made, not when creating a model, so one model or registry entry works across stacks and catalogs. Selection is independent of helper order and never serialized into ProviderWire options or headers. No option means the existing namespace routes.

Support the option only with `NewWithTokenExchange`. The other constructors keep accepting catalog-prefixed base URLs. Refuse the option with either direct credential flow rather than silently use a different catalog or interpret an arbitrary configured prefix.

### Build the prefix once for every operation

Preserve `NewWithTokenExchange`'s API-root validation, including refusal of a path segment named `namespaces`. Resolve the namespace using the existing rules and validate the catalog before any exchange. In the shared request builder, append `/namespaces/<route-namespace>/modelcatalogs/<name>` to the root, then use the existing `GET /config` or `POST /language-model` suffix. Preserve any base path ahead of the namespace. A request for a selected catalog must not fall back to the namespace route.

- Concrete configured namespace: use it by default; an explicit `WithNamespace` must match. With no configured namespace, the call still needs `WithNamespace`.
- A concrete exchange uses the route namespace and the existing on-behalf-of subject, if supplied. Send only the resulting token as `X-Access-Token`. Catalog selection does not affect exchange caching or sharing (still keyed by exchange namespace and subject), or derive routing from decoded token claims.
- Keep the existing wildcard case: a service acting as itself still selects a concrete route namespace and exchanges for `*`; an on-behalf-of call still requires a concrete exchange namespace. Let Grafana enforce access to the selected resource, without any new client permission logic.

For example, selecting catalog `team.eu` and stack `stacks-123` for a user exchanges for `stacks-123` with that subject, then sends the result to `.../namespaces/stacks-123/modelcatalogs/team.eu/config` for `ListModels`, or `.../language-model` for generation or streaming.

### Validate the path segment; leave resource names to Grafana

Reject an empty name, `.` or `..`, and names containing `/`, without echoing the value. Encode all other names with `url.PathEscape` before appending the segment, preserving the input without trimming or case folding. The option accepts a raw name, not an already escaped path. The DNS-label check in the Gateway is for provider keys, not catalog names, and mirroring Grafana's storage validation would let the client drift from the server. Grafana decides whether the resulting resource name is valid and exists.

### Upstream alignment and evidence

`test/conformance/upstream.yaml` pins Vercel AI SDK `ai@7.0.173`, `@ai-sdk/gateway@4.0.146`, and `@ai-sdk/provider@4.0.38` at `0e8e0543aeda7d4dd4ce6eade0d5b7d5a43644c1`. At that revision, `createGateway` fixes the base URL at construction and resolves an API key or OIDC bearer. `GatewayFetchMetadata.getAvailableModels` appends `/config` and `GatewayLanguageModel.getUrl` appends `/language-model` for both model operations. There is no upstream per-call Grafana namespace, stored catalog, or exchange subject.

This is a Go extension, not a missing upstream parity feature. `test/conformance/PARITY.md` covers `grafana-gateway-client` in the **AI Gateway** row (Go extension, mixed); **ProviderWire request projection** has automated goldens. Add a Go transport-level request snapshot (method, path, selected protocol headers and JSON body) with synthetic credentials. The stock client goldens and the existing fixed-prefix catalog differential remain regression checks; no upstream or recorded fixture changes are needed. A mocked exchange proves token routing and subject handling, not live Grafana issuance or permissions.

## Open Questions for Review

| Question | Recommendation |
| --- | --- |
| How should callers select a catalog? | Use `WithCatalog` as a context helper, not a constructor setting, model view, or serialized provider option. Use the same context for discovery and inference. |
| Can an explicit empty selection clear an inherited catalog? | No. Mirror namespace validation: reject empty locally; use a context without a selection for namespace routes. |
| Should direct credential flows also accept the option? | No. Keep their explicit catalog-prefixed URLs; fail before I/O if they receive a catalog selection. |
| Does selecting a catalog require a concrete exchange even for services? | Keep the existing wildcard routing behavior for services acting as themselves. Never allow wildcard exchange on behalf of a user. |
| Which name validation and error contract should be public? | Validate only path-segment safety and encode the raw name. Let Grafana validate the resource name; do not add a public error type or sentinel. |
| How much new cross-boundary evidence is needed? | Add focused Go exchange/transport coverage and snapshot, and run the existing ProviderWire suite. Do not expand the platform cases, TypeScript goldens, or live exchange environments for this change. |

## Compatibility

The only new public symbol is the context helper. Existing consumers do not adopt it implicitly. No migration or generated release-file edits are needed.
