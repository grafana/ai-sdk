# Design

## Context

- **Not parity-sensitive;** see the proposal.
- **This change replaces #778's earlier egress-policy design** before release. That design had:
  - an egress client with no proxy, a resolving dialer that checked every address against IP ranges, and allow and deny lists;
  - sanitized connection errors;
  - three egress sentinels with a `CategoryEgressRefused` and its 424 documents.

  None of these exist on `main` (`git grep` on `origin/main` finds no egress symbols), so they can be deleted without affecting other PRs. `CategoryCatalogConfiguration` and its documents belong to `main` and stay.
- **Kept from #778:**
  - stack catalogs accepting `baseURL` and `openai-compatible` fields;
  - candidates built with `BaseURL`, `ProviderName` and `SupportsStructuredOutputs`;
  - the shared `providerSlug` rule;
  - BYOK decoding storing a normalized `baseURL`;
  - the native-default rule (`nativemodel.DefaultBaseURL`);
  - the URL form check (`outbound.ValidateEndpoint`);
  - the e2e TLS listener on `fake-anthropic`.
- **Connections:** `outbound.NewModelClient` (`ProxyFromEnvironment`, bounded bodies, redirect refusal) serves the operator catalog, BYOK and stack catalogs on `main`. It will serve custom endpoints too.

## Goals / Non-Goals

**Goals:**

- One string rule for both customer sources, implemented once and checked where configuration is saved or decoded.
- No network policy in the Gateway: the operator's list is the trust boundary.

**Non-Goals:**

- Wildcard or pattern entries, per-namespace lists, a separate list per source.
- Checking the operator catalog's endpoints.

## Decisions

### The endpoint list (`outbound`)

- `outbound.CustomEndpoints` is built by `ParseCustomEndpoints(raw string) (CustomEndpoints, error)`. Each comma-separated, trimmed entry must pass these checks:
  - `ValidateEndpoint` (absolute, hierarchical, a host, no userinfo, query or fragment, `http` or `https`);
  - an ASCII host, with no `*`;
  - no backslash, and no path segment that decodes to `.`, `..` or a value containing `/`, `\` or `%`. The Anthropic and OpenAI SDKs set `u.Path += "/"` on the parsed base, which drops the original escaping, then resolve their own path with `URL.Parse`, which removes dot segments: `.../team/x%2F..%2F..%2Fadmin` would otherwise request `/admin/v1/messages`. A `%` left after one decoding is refused too, so a server that decodes twice can't see `..`. Candidate hosts must be ASCII as well, because `strings.ToLower` maps characters such as the Kelvin sign to ASCII.

  Entries are stored normalized: lowercase scheme and host, the port as written, the escaped path without a trailing `/`.
- The parse fails on an empty item, a duplicate after normalization, an invalid entry, or more than 256 entries. Messages never contain the value. The 256 bound is the one the earlier lists used.
- `CustomEndpoints.Check(raw, native string) (string, error)` returns:
  - `""` when `raw` matches `native`. This rule is kept unchanged: scheme and host compare case-insensitively and one trailing `/` is ignored.
  - Otherwise, the URL as parsed when it matches an entry. That is the value the provider SDK receives, so matching and connecting use the same host, port and path.
  - Otherwise one of two sentinels: `ErrEndpointForm` (form, non-ASCII host, backslash, dot segment or encoded separator) or `ErrEndpointNotListed`.
- Match rules:
  - scheme equal;
  - host equal, case-insensitively;
  - port string equal;
  - candidate path `p` and entry path `e`, both without a trailing `/`: `p == e`, or `e == ""`, or `p` starts with `e + "/"`.
- There is no DNS lookup, no IP parsing and no host-form check. A numeric or alternative host form matches only if the operator listed it in that exact form.

Alternatives considered:

- **Default-port equivalence** (`:443` equals no port): the same destination, but nara asked for exact matches, and the native rule treats `:443` as custom. See Open Questions.
- **A URL-prefix string comparison:** it would match `/v10` against `/v1`, so the match is on segment boundaries instead.

### Where it is checked

| Check | Code | Failure |
|---|---|---|
| Stack-catalog admission | `stackcatalog.Parse(spec, keys, endpoints)`, from `Validator{Endpoints}` | `ErrEndpointForm`: `modelcatalog: providers.<n>.baseURL must be an absolute URL without userinfo, query, fragment, dot segments or encoded separators`. `ErrEndpointNotListed`: `modelcatalog: providers.<n>.baseURL is not a listed custom endpoint`. Kubernetes 400. |
| Every stack-catalog request | `parseStackCatalog(stack, endpoints)` in `SelectStackCatalog` and `StackCatalogLister` | bare `execution.ErrCatalogConfiguration`: ProviderWire 424 "model catalog is invalid or missing a provider key"; Chat Completions 424 `failed_dependency` (main's mapping) |
| BYOK decode | `byok.DecodeRequest(selector, controls, endpoints)` | `ErrInvalidRequest` → `execution.ErrInvalidBYOK`: the fixed BYOK credentials 400 |

- An entry removed from the list applies after a restart, on the next call: settings are read at startup, and stored catalogs are checked on every request.
- The `https`-only rule and its message (`baseURL must use https`) go away. The scheme now comes from the entry.

### Connections

- Custom endpoints use the shared model client, so `HTTPS_PROXY`, `NO_PROXY` and `SSL_CERT_FILE` apply to them as to native endpoints.
- `StackCatalogOptions{Client, Endpoints}` and `BYOKOptions{Client, Endpoints, Factory}` go back to one client. `outbound.ModelClients`, `EgressModel`, `NewEgressClient` and the `newTransport` parameters are deleted, and `outbound.go` returns to `main`'s version.
- Transport errors of a listed endpoint classify and render like any provider's (upstream, timeout). Their text can name the resolved address, as native and operator endpoints already can. The listed endpoints are operator-trusted, so the earlier sanitizing isn't needed.

### Setting

- `custom_endpoints` (`GF_PLUGIN_CUSTOM_ENDPOINTS`): `config.CustomEndpointsSetting`, and `Settings.Endpoints outbound.CustomEndpoints`.
- `custom_endpoint_allow` and `custom_endpoint_deny` are removed and ignored like other removed settings. They were never released.
- `provisioning/grafana.ini` declares `custom_endpoints =`, so e2e overrides apply.

### Deleted

| Area | Deleted |
|---|---|
| `outbound` | `egress.go`, `egress_client.go` and their tests; `ModelClients`, `EgressModel`; the transport refactor in `outbound.go`. `golang.org/x/net` returns to indirect. |
| `execution` | `ErrEgressRefused`, `ErrEgressConnection`, `ErrEgressTimeout`, `CategoryEgressRefused` and the `classifyTransport` case |
| `providerwire/v4` | `safeEgressRefused`, its document and `TestEgressRefusalClassification` |
| `openai/chatcompletions` | the `CategoryEgressRefused` mapping and its test cases; main's catalog-configuration mapping stays |
| `service`, `gateway`, `config` | the egress client wiring, the policy fields, the allow and deny settings, and the egress harness tests (`TestCatalogRoutes_RefusedEndpoints` and `TestBYOK_RefusedEndpoints` become list tests) |
| e2e | network aliases `egress-refused` and `egress-denied`, their certificate SANs and the per-host counter |
| docs | the egress README section, the glossary entry and the architecture sentence, all rewritten for the list |

Estimated: about −2,000 lines deleted and +600 written. #778's diff against `main` shrinks from about +2,900/−240 to about +1,500 outside OpenSpec, and the two archived changes are replaced by one.

## Test plan

| Behavior | Evidence |
|---|---|
| Parsing | `ParseCustomEndpoints` table:<br>valid: `https` and `http` entries, with and without paths, IPv6 literals;<br>invalid: userinfo, query, fragment, `*`, non-ASCII host, dot segments, empty items, duplicates after normalization, 257 entries;<br>messages carry no value. Settings tests for the key; the old keys are ignored. |
| Matching | `TestCustomEndpoints_AcceptedEndpointIsTheDestination` drives all three provider SDKs with every accepted candidate and asserts the requested URL stays on the entry's origin and under its path; a mutation that drops the encoded-separator rule fails it. `Check` table:<br>path equal, trailing `/`, deeper path, segment boundary (`/v10` against `/v1`), root entry;<br>scheme mismatch, port as written (`:443` against none), host case, another host name;<br>dot segments literal and `%2e`, encoded `%2F`;<br>native rule with an empty list, and a native host with another path;<br>an empty list refuses every custom endpoint. |
| Stack admission | `stackcatalog` `Parse` and `Validator`: listed endpoint accepted on all three types, unlisted refused with the field-path message and no URL, compatible fields accepted. Gateway admission through the test host. |
| Stack request time | `service`: a catalog whose endpoint isn't in the options' list fails with bare `ErrCatalogConfiguration` and zero builds. The lister fails the same way. |
| BYOK decode | `byok` decode table: listed accepted and normalized, unlisted refused (unused provider included), native unset, `http` only when listed, no URL in messages. The service maps it to `ErrInvalidBYOK`. |
| Connections | `service` and `byok` recording-client tests: a listed endpoint uses the shared client with the account or catalog key. Gateway harness in `cloud` mode with a listed loopback TLS fake: stack-catalog and BYOK calls, unary and stream, reach it; an unlisted endpoint gets the 400 or 424 with zero requests at the fake. |
| Operator catalog unaffected | Existing harness tests with the loopback `http` operator endpoint and an empty list. |
| Cross-language | `test/providerwire-v4`: both clients get the BYOK 400 for an unlisted account endpoint, with zero connections to its listener. The stack-catalog scenario is refused at admission by the test host. |
| e2e | `tests/catalog-egress.spec.ts` becomes `tests/catalog-endpoints.spec.ts`, with the e2e-only entry `https://fake-anthropic:8443/team`, still 4 tests:<br>1. a listed endpoint serves unary and streamed calls with the catalog key;<br>2. admission refuses an unlisted host, another port, `http`, a path outside the prefix (`/teams`) and a dot segment;<br>3. an `openai-compatible` catalog at `.../team/v1` is accepted and discovered by its `providerName`;<br>4. an explicit native `baseURL` is accepted with the list in place.<br>`catalog.spec.ts` keeps its unlisted-`baseURL` refusal. Playwright stays at 26. A BYOK Playwright case still needs a cloud-mode Grafana, so BYOK is proven by the plugin-protocol suite. |

## Risks / Trade-offs

- **[A listed hostname trusts its DNS]** Whoever controls the name controls the destination, including internal addresses. Documented: list only endpoints you control, or vendors whose DNS you trust.
- **[Cloud mode widens who can use a listed endpoint]** Any user or service account can use it through BYOK, with its own key. Documented.
- **[Listed `http` entries send keys in cleartext]** That's the operator's explicit choice. Documented.
- **[Proxy-only deployments]** Custom endpoints now go through the proxy like native ones, which removes the earlier limitation.
- **[Error text may name a listed endpoint's resolved address]** This is the same as native and operator endpoints, and the endpoints are operator-trusted.

## Migration Plan

- Unreleased: operators who tried the branch rename their lists to `custom_endpoints` and list endpoints instead of address ranges.
- Rolling back to `main` makes admission refuse `baseURL` again. Stored catalogs with custom endpoints then fail per request with the catalog configuration document.

## Follow-ups for #759

- The catalog form needs `baseURL`, the `openai-compatible` type, `providerName` and `supportsStructuredOutputs`. It could show the operator's list when one exists.

## Open Questions

Each has a recommendation; the specs and tasks follow it.

1. **Ports:** compare as written (recommended, nara's "exact" and consistent with the native rule), or treat a scheme's default port as equal to none.
2. **Dot segments:** refuse `.`/`..` path segments in entries and endpoints (recommended; they would defeat the path prefix at servers that normalize), or compare raw paths only.
3. **Hosts:** ASCII only, compared lowercase, with no trailing-dot or IDN normalization (recommended; non-matches are refusals, so this fails safe), or normalize them.
4. **Format:** comma-separated like `access_internal_services` (recommended), or a YAML list.
5. **Error sanitizing:** delete it, as instructed (recommended; the endpoints are operator-listed), or keep the connection-error rewrite for customer-facing overviews.
6. **`outbound.go`:** revert the transport refactor to `main`'s version (recommended; it existed only for the egress client).
7. **OpenSpec shape:** one new change `add-custom-provider-endpoints` with capability `gateway-custom-endpoints`, with both earlier archives and the `gateway-stack-catalog-egress` main spec dropped (recommended; #778 then ends with exactly one archived change).
