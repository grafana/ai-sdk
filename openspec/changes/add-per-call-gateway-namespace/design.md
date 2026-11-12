# Design

## Context

See proposal.md for motivation. Current state in `providers/grafana`:

- `TokenExchangeConfig` carries `BaseURL` with `/namespaces/<namespace>` and a required `Namespace`, so the stack appears twice and can disagree.
- `Provider.request` prepares every call: it validates call headers, reads the on-behalf-of subject from the context (`WithOnBehalfOf`), refuses on-behalf-of calls without exchange or with `*`, exchanges, and builds `baseURL + route`.
- `Provider.exchange` shares concurrent exchanges with `singleflight` keyed by subject only, and calls the authlib exchanger with the provider's fixed namespace and audience.
- authlib's `TokenExchangeClient` at the pinned version already caches tokens per request: its cache key is namespace, sorted audiences and subject identity (`TokenExchangeRequest.hash`). Only the client's own sharing key ignores the namespace.
- The exchanger is built with the provider's HTTP client (`authn.WithHTTPClient(p.client)`), so a caller-supplied `HTTPClient` with a custom root CA applies to both the exchange and Gateway requests, and redirects are refused for both. authlib sets `X-Org-ID: 0` and the system `X-Realms` header on every exchange, which system access policies require.

## Goals / Non-Goals

**Goals:**

- One provider serves any number of stacks, with per-stack token isolation and no extra HTTP or cache setup.
- A call can never reach a stack other than the one its caller selected; ambiguity fails before I/O.

**Non-Goals:**

- Per-call namespaces for service-account or caller-managed access-token providers. Those credentials are minted for one stack.
- Per-call audience or exchange URL.
- Any change to the Gateway's caller access, allowlist or delegated-token verification.

## Decisions

### Shape: a context option, `grafana.WithNamespace(ctx, namespace)`

The call namespace travels in the context, like `WithOnBehalfOf`. Models stay stack-agnostic, so one `LanguageModel` value works through `GenerateText`, `StreamText`, agents, fallback and registries without changes outside the provider. A service serving a request for one stack sets the namespace once where it already sets the on-behalf-of token.

Alternatives:

- A per-namespace view, `p.ForNamespace(ns)`, sharing the exchanger and client. Explicit at the type level, but models become stack-bound again, so callers still map stacks to model values, and registries built once at startup cannot be reused across stacks.
- A provider option in `CallOptions.ProviderOptions`. Provider options are part of the ProviderWire body, so a routing input would leak into the wire format.

### Base URL: the Gateway API root for the exchange flow

`TokenExchangeConfig.BaseURL` is the API root, for example `https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1`, and the client builds `<root>/namespaces/<namespace>/config` and `.../language-model`. The namespace then has one source of truth for both route and exchange. A path segment named `namespaces` in the base URL fails construction, so a namespaced URL left over from the previous shape cannot silently produce `/namespaces/a/namespaces/b`.

Service-account and access-token constructors keep the namespaced route prefix shown on the Gateway's overview page; their credential already fixes the stack.

Alternatives:

- Keep the namespaced URL and switch to a root URL only when `Namespace` is empty. One field would change meaning depending on another, and the misconfiguration would only surface as a 404.
- A separate root-URL field next to `BaseURL`. Two mutually exclusive fields add surface without adding a capability.

### Namespace resolution

| Configured `Namespace` | No call namespace | Call namespace `N` |
|---|---|---|
| Concrete `C` | route and exchange `C` | `N == C`: as without; otherwise conflict |
| Empty | fails: namespace required | route and exchange `N` |
| `*` | fails: namespace required | route `N`, exchange `*` |

- `*` keeps its meaning, a token for every namespace, so one exchanged token serves every stack for calls a service makes as itself. The Gateway attributes such calls to the route namespace.
- On-behalf-of calls need a concrete exchange namespace; with `*` they still fail before any I/O.
- A call namespace on a service-account or access-token provider is a conflict. Ignoring it would send the call to the provider's fixed stack while the caller believes it selected another.

### Namespace grammar: DNS label

Configured concrete and call namespaces must be RFC 1123 labels: 1 to 63 lowercase alphanumerics or `-`, starting and ending alphanumeric. That is the platform's namespace grammar (`default`, `stacks-<id>`, `org-<id>`), and it makes the value safe as a single path segment without escaping. `*` is accepted only as a configured value. Invalid values fail construction or the call before I/O.

### Exchange sharing per namespace and subject

The `singleflight` key becomes the exchange namespace and subject, separated by a byte that the namespace grammar excludes. Concurrent calls for different stacks never share an exchange, and authlib's cache already keys tokens by namespace. The exchange keeps its existing detached timeout and per-caller cancellation.

### Audience: the Gateway's API group in Grafana Cloud

Evidence from dev validation through Grafana's router, with an access token signed from an internal service's access policy:

| Audience | Result |
|---|---|
| `aigateway.ext.grafana.app` (API group) | accepted; an allowlisted service gets the configured catalog |
| `grafana` | accepted; same result |
| `grafana-aigateway-app` (plugin ID, documented today) | refused at token signing; the access policy does not allow it |

From code, not tested: the router's access-token verification accepts its configured audiences or the request's API group, so the plugin ID would not verify there even if signed.

The godoc and guide document `aigateway.ext.grafana.app` for Grafana Cloud callers, internal services and App Platform apps alike, and state that the caller's access policy must allow it. `grafana` also works, but grants a token every Grafana API accepts; the API group limits the token to the Gateway. Self-hosted App Platform guidance keeps `grafana`. The audience stays required with no default.

### Upstream alignment

Registered baseline: `@ai-sdk/gateway` 4.0.132 (`test/conformance/upstream.yaml`). Its `createGateway` fixes `baseURL` at construction (default `https://ai-gateway.vercel.sh/v4/ai`) and resolves an `Authorization` bearer from `apiKey` or OIDC per request; it has no token exchange, namespace or per-call route selection.

Classification: **Go extension** of the Grafana client's construction and authentication, with no upstream counterpart. The ProviderWire request body, client-owned headers, SSE framing, error envelopes and discovery documents are unchanged; only the namespace path segment and the exchanged credential vary per call. No conformance fixtures, `knownGaps` entry or `PARITY.md` row change; the AI Gateway row already covers the client specs.

## Risks / Trade-offs

- [A context value is implicit and easy to forget] → Calls without a resolvable namespace fail before I/O with a stable error, and a namespace reaching a provider that cannot honor it fails instead of being ignored.
- [Breaking `BaseURL` meaning for existing exchange callers] → Construction refuses a base URL with a `namespaces` segment, so the old shape fails loudly; the guide and godoc show the new one.
- [Tighter namespace grammar may refuse a previously accepted value] → Platform namespaces are DNS labels; anything else could not have been routed correctly.
- [A wildcard token serves every stack] → Unchanged risk profile from today's `*`; documented as a choice for calls a service makes as itself.

## Migration Plan

Existing `NewWithTokenExchange` callers drop `/namespaces/<namespace>` from `BaseURL`. Single-stack callers keep `Namespace`; multi-stack callers leave it empty and add `WithNamespace` per call. The Grafana client is a separate module in the alpha channel, so the change is released as a breaking change that advances its prerelease counter.
