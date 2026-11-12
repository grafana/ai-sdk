# Proposal

## Why

Stack catalogs can only reach the native `anthropic` and `openai` endpoints, so a stack can't use a regional endpoint, a hosted proxy or an `openai-compatible` backend. In a stack catalog the customer chooses the URL, but the request leaves from the Gateway's pods, so an arbitrary URL is a server-side request forgery (SSRF) vector: it could reach cloud metadata, cluster services or other internal addresses. `outbound.ValidateEndpoint` checks a URL's form, not where its host resolves. Custom endpoints need an egress policy that is enforced when connecting.

## What Changes

- **Egress policy for custom stack-catalog endpoints.** A stack-catalog provider with a `baseURL` connects through a dedicated egress client:
  - HTTPS only;
  - every address the client connects to is checked at the socket after DNS resolution. Loopback, private, link-local, CGNAT, unspecified, multicast, documentation, benchmarking, reserved and other non-public IPv4 and IPv6 ranges are refused, including IPv4-mapped and NAT64-embedded IPv4 addresses. DNS rebinding can't bypass the check;
  - no environment proxy (`HTTPS_PROXY`), so the dialer always sees the real destination;
  - its own connection pool, so it never reuses a connection opened for the operator catalog;
  - redirects are not followed, and the response bounds of the shared model client apply.
- **Operator allow and deny lists.** Two settings, `stack_catalog_egress_allow` and `stack_catalog_egress_deny`: comma-separated IP addresses, CIDR prefixes, hostnames or `*.` domain wildcards. A deny entry always wins. An allow entry lifts the built-in refusal for matching destinations, except unspecified and multicast addresses. Invalid entries fail startup.
- **Stack catalogs accept `baseURL` and `openai-compatible`.** Admission and per-request checks accept `type: openai-compatible` with `baseURL`, `providerName` and `supportsStructuredOutputs`, and `baseURL` on `anthropic` and `openai`, under the operator catalog's shape rules. The `baseURL` must be `https` and pass the endpoint form checks. Its host must be a DNS name or an IP literal, not a numeric form such as `2130706433`. An IP literal or a deny-listed host is refused statically. No DNS lookup is made at admission.
- **Refusal without a request or an address.** A refused connection fails before any byte is sent. The refusal and DNS failures surface as one fixed error that names no address, host, resolver or range. The call gets a new `egress refused` category with its own fixed ProviderWire document (424, `failed_dependency`, not retryable). Like any connection failure, a refused candidate falls back to the next one.
- **Discovery and routing identity.** Stack-catalog candidates use the operator rule for their provider slug: an `openai-compatible` provider's `providerName`, otherwise the type.
- **Unchanged.** The operator catalog's endpoint rules and client, BYOK, and stack-catalog providers without a `baseURL`, which keep the native endpoint through the shared model client.
- **Docs.** README guidance on custom stack-catalog endpoints, the egress policy, its settings and its limits.

## Capabilities

### New Capabilities

- `gateway-stack-catalog-egress`: the egress policy for custom stack-catalog endpoints: scope, HTTPS, refused ranges, connect-time enforcement, proxy handling, the operator lists, static checks, the refusal error and its evidence.

### Modified Capabilities

- `gateway-model-catalog-kind`: stack catalogs accept `baseURL`, `providerName`, `supportsStructuredOutputs` and `openai-compatible` under the egress policy's static checks, instead of refusing them.
- `gateway-stack-catalog-runtime`: provider slugs follow the operator rule for `openai-compatible` providers.
- `gateway-configured-discovery`: catalog route discovery reports the same provider slug.
- `gateway-provider-configuration`: stack-catalog keys go to the catalog's `baseURL` through the egress client when one is set. The OpenAI and compatible transport requirements name both clients.
- `gateway-resource-policy`: the outbound bounds apply to the shared model client and the egress client alike.
- `gateway-execution-runtime`: the closed failure classification gains `egress refused`.
- `providerwire-v4-unary-runtime`: a fixed document for `egress refused`.

## Impact

- **Code:**
  - `ai-gateway/pkg/internal/outbound`: the egress policy, its address classification and the egress client;
  - `ai-gateway/pkg/internal/config`: the two settings;
  - `ai-gateway/pkg/internal/stackcatalog`: policy, conversion and the validator, which now takes the egress policy;
  - `ai-gateway/pkg/internal/service`: the stack-catalog builder, the client choice and the provider slug;
  - `ai-gateway/internal/execution` and `ai-gateway/providerwire/v4`: the category and its document;
  - `ai-gateway/pkg/internal/gateway`: wiring.
- **APIs:** no schema change to the `ModelCatalog` kind; the fields already exist. A new ProviderWire error document with a registered status, type and code. Two new plugin settings.
- **Tests:** Go unit and harness tests with no external network; `ai-gateway/test/providerwire-v4` client scenarios; Playwright specs and e2e infrastructure (a TLS listener on the fake provider, network aliases, an e2e-only allow entry).
- **Docs:** `ai-gateway/README.md`; the architecture walkthrough and glossary if their outbound description drifts.
- **Not parity-sensitive:** the Vercel AI SDK has no tenant catalog or egress policy, and no stream, chunk, provider request or wire format changes beyond a Gateway-owned error document.
