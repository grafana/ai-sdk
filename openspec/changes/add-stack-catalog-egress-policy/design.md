# Design

## Context

- **Not parity-sensitive** (see proposal). `test/conformance/PARITY.md` routes the AI Gateway to `gateway-*` specs; no conformance fixture or upstream package is involved.
- **One shared model client.** `outbound.NewModelClient` serves the operator catalog, BYOK and stack catalogs. It is an `http.Transport` with `Proxy: http.ProxyFromEnvironment` and a plain `net.Dialer`, wrapped in `boundedTransport`, with `CheckRedirect: rejectRedirect`. `outbound.ValidateEndpoint` checks URL form only (absolute, host, no userinfo, query or fragment, `http` or `https`).
- **Operator endpoints.** `gateway.validateProviderEndpoints` runs `ValidateEndpoint` at startup and normalizes `baseURL`. The operator catalog may use `http` and private addresses; e2e uses `http://fake-anthropic:8080`.
- **Stack-catalog checks.** `stackcatalog.checkPolicy` refuses `baseURL`, `providerName`, `supportsStructuredOutputs` and any type other than `anthropic`/`openai`. `convert` copies only `Type`. `selectStackCatalog` builds `config.ResolvedProvider{Type, APIKey}`, so endpoint and compatible fields never reach `newCandidate`. `StackCatalogLister` and `stackIndex.resolve` use the type as the provider slug, while `newCandidate` uses `providerName` for `openai-compatible`.
- **Admission.** `stackcatalog.Validator{}` is a zero value built in `managedKinds()`, for running and idle Gateways. An idle Gateway refuses admission reviews, so only a running Gateway needs the policy.
- **Errors.** `execution.summarize` copies a non-API error's `Error()` text into the overview's attempt failure, and providers wrap transport errors with their text. `openai-compatible` wraps them in a retryable `APICallError` with `Cause`. A `net.OpError` names the dialed address, and a `net.DNSError` names the resolver. Today, a refusal at the dialer would therefore leak addresses into the overview.
- **Classification.** `ClassifyProvider` maps an `APICallError` by status (0 → upstream), then transport errors. ProviderWire maps each category to one fixed document; the Go and TypeScript clients already classify 424 `failed_dependency` as not retryable.
- **e2e.** Grafana's environment sets `HTTPS_PROXY=http://native-proxy:8443` and `SSL_CERT_FILE` to a run-time CA whose SANs are the native hosts. Stack catalogs reach `api.anthropic.com` through that proxy. `fake-anthropic` listens on plain HTTP 8080 on the Docker network, a private address.
- **Settings.** Plugin settings are `GF_PLUGIN_<KEY>` variables. Lists use comma-separated entries, as `access_internal_services` does.

## Goals / Non-Goals

**Goals:**

- A connect-time check that no resolver, rebinding, numeric host form, proxy, pooled connection or redirect can bypass.
- A static check at admission and per request that catches the obvious cases with a field-path reason, and never resolves DNS.
- Refusal errors that carry no internal address in any surface.
- The operator catalog, BYOK and native stack-catalog endpoints keep their client and behavior.

**Non-Goals:**

- An egress policy for the operator catalog or BYOK.
- Port restrictions, rate limits on custom endpoints, or an exclusive allow-list mode.
- A trusted egress proxy for custom endpoints (see Decisions for review).
- Frontend changes; #759 owns the catalog UI.

## Decisions

### Where the check sits: dialer `Control`, behind a host check

`outbound.NewEgressClient(headerTimeout, responseBytes, policy)` returns a client with its own `http.Transport`:

- `Proxy: nil`.
- `DialContext` receives the unresolved `host:port`, since there is no proxy:
  1. refuses a deny-listed hostname before resolution;
  2. records whether the host matches an allow hostname entry;
  3. dials through a `net.Dialer` whose `Control` checks the address being connected.
- `Control` runs after resolution, on the socket, before `connect()`, for every attempt, including Happy Eyeballs fallbacks across address families. It parses the address as a `netip.AddrPort` and applies the policy. A refusal returns the sentinel, so the refused address receives no packet.
- A refused address makes the dialer try the next one. A host with mixed records can therefore only ever connect to a permitted address.
- `RoundTrip` refuses any request whose scheme isn't `https` before dialing. It then wraps the transport in the same `boundedTransport` with `rejectRedirect`.
- Same timeouts, idle-connection and per-host limits as the shared transport, built by one shared constructor that takes the proxy and dial functions.

Alternatives considered:

- **Resolver-level check:** resolve, filter, then dial the IP. It duplicates the dialer's address racing, and a later dial could resolve again.
- **`RoundTrip` URL check only:** it never sees resolved addresses, so it can't stop rebinding.
- **Validation-time DNS:** subject to rebinding by definition, and an oracle for internal names.

### Refused ranges

A pure `classify(netip.Addr) decision` over the normalized address. Zones are refused; an `Is4In6` address is unmapped first.

| Family | Refused |
|---|---|
| IPv4 | `0.0.0.0/8`, `10.0.0.0/8`, `100.64.0.0/10` (CGNAT), `127.0.0.0/8`, `169.254.0.0/16` (link-local, cloud metadata), `172.16.0.0/12`, `192.0.0.0/24`, `192.0.2.0/24`, `192.88.99.0/24`, `192.168.0.0/16`, `198.18.0.0/15`, `198.51.100.0/24`, `203.0.113.0/24`, `224.0.0.0/4` (multicast), `240.0.0.0/4` (reserved, broadcast) |
| IPv6 | everything outside `2000::/3`: `::`, `::1`, IPv4-compatible `::/96`, `100::/64`, `fc00::/7` (incl. `fd00:ec2::254`), `fe80::/10`, `fec0::/10`, `ff00::/8`, `5f00::/16` and other unassigned space. Inside `2000::/3`: `2001::/23` (incl. Teredo, benchmarking, ORCHID), `2001:db8::/32`, `2002::/16` (6to4), `3fff::/20` |
| Embedded | `::ffff:0:0/96` and `64:ff9b::/96` are checked as their IPv4 address. `64:ff9b:1::/48` is refused |

- Built as an allow-shape for IPv6 (global unicast only) plus explicit IPv4 prefixes, so new IANA assignments outside `2000::/3` stay refused.
- NAT64 is checked by its embedded IPv4 address, not refused outright. That keeps IPv6-only clusters with DNS64 working for public IPv4 providers.
- "Never dialable" means unspecified (`0.0.0.0`, `::`) and multicast (`224/4`, `ff00::/8`). No allow entry lifts these.

### Proxy handling: direct connections only

The egress client ignores the environment proxy. With a proxy, the dialer only sees the proxy's address, and the proxy resolves the destination, so no Gateway-side check can hold. The proxy variables stay in effect for the shared client, so native endpoints, BYOK and the operator catalog keep using the proxy. If a deployment allows egress only through a proxy, custom stack-catalog endpoints can't connect there. The README states this.

Alternatives considered:

- **Honor the proxy and check only its address:** the destination is unchecked.
- **Pre-resolve and `CONNECT` to the checked IP literal:** many proxies refuse IP `CONNECT`, and the policy then depends on the proxy's DNS view matching the Gateway's.
- **An operator-designated trusted egress proxy** that enforces the policy itself: possible later, when a deployment needs it (see Decisions for review).

### Which candidates use the egress client

- `service.StackCatalogOptions` gains `EgressClient *http.Client`. `selectStackCatalog` passes `BaseURL`, `ProviderName` and `SupportsStructuredOutputs` into `config.ResolvedProvider`. The stack builder calls `newCandidate` with the egress client when `BaseURL != ""`, and the shared client otherwise.
- `newCandidate` is unchanged: it already handles all three types and the compatible fields.
- Native stack-catalog endpoints keep the shared client. Their destination is fixed by the Gateway, not the catalog, and the #758 e2e relies on its proxy.
- One egress client per Gateway, shared by all stack catalogs. The policy is process-wide and the pool is keyed by host and port, so reuse is only for a host that passed the same checks. It is never shared with the operator catalog's pool: an idle operator connection to an internal host must not serve a stack catalog.

### Operator setting: format and precedence

- `stack_catalog_egress_allow` and `stack_catalog_egress_deny`, parsed in `config.ParseSettings` by `outbound.ParseEgressPolicy(allow, deny)`. `Settings` gains `Egress outbound.EgressPolicy`.
- Entries are comma-separated and trimmed. Each entry is one of:
  - an IP address, as `/32` or `/128`;
  - a CIDR prefix, which must be canonical (`netip.Prefix.Masked() == prefix`) and plain IPv4 for IPv4;
  - a hostname: ASCII, an RFC 1123 subdomain, lowercased, without a trailing dot;
  - `*.` and a hostname, which matches subdomains, not the apex.
- Empty, duplicate, malformed, non-canonical or IPv4-mapped entries fail startup with `config: GF_PLUGIN_STACK_CATALOG_EGRESS_ALLOW has an invalid entry`, without the value. At most 256 entries per list.
- Evaluation, for host `h` and each dialed address `a`:
  1. scheme not `https` → refused (not configurable);
  2. `h` matches a deny hostname entry, or `a` is in a deny prefix → refused;
  3. `a` is unspecified or multicast → refused;
  4. `h` matches an allow hostname entry, or `a` is in an allow prefix → permitted;
  5. `a` is in a refused range → refused;
  6. otherwise permitted.
- Deny always wins. Allow is a list of exceptions, not an exclusive list. An allow hostname entry trusts that name's DNS; the README recommends prefix entries.

Alternatives considered:

- **One YAML or JSON setting:** no other setting uses structured values, and two lists stay readable in `grafana.ini`.
- **An exclusive allow mode:** no requirement yet, and it can be added as a separate setting.
- **Prefix-only entries:** hostname entries are what self-hosted operators can name, and e2e needs one, because Docker assigns addresses at run time.

### Static checks: admission and per request

- `stackcatalog.Parse(spec, keys, policy)`. `checkPolicy` accepts the three types and the compatible fields, and `convert` copies them. `File.ValidateShape` keeps owning the type and field rules: compatible requires `baseURL`, only compatible sets the other two, and the reserved `providerName` namespace. Then, for each `baseURL`:
  1. `outbound.ValidateEndpoint`;
  2. scheme `https`;
  3. host check: an IP literal, or an ASCII DNS name whose last label is not all digits. This refuses `127.1`, `2130706433` and `0x7f000001`, which some resolvers read as IPv4;
  4. `policy.CheckHost(host)`: deny hostname entries, and the full address rules for IP literals.
- No DNS lookup.
- Messages name the field path only, for example `modelcatalog: providers.local.baseURL is refused by the egress policy`. They never echo the URL, host or matched entry.
- `Validator` gains a `Policy outbound.EgressPolicy` field. `Gateway` keeps the parsed policy, and `managedKinds` passes it; an idle Gateway passes the zero policy, which is never used.
- Request time: `parseStackCatalog` passes the runtime policy. Failures remain the bare `ErrCatalogConfiguration`, so a catalog saved before a stricter policy fails with the catalog configuration document.
- The normalized `baseURL` (`parsed.String()`) is used, as for the operator catalog.

### Error mapping

- `execution.ErrEgressRefused` (`"gateway outbound: provider endpoint refused by the egress policy or not resolvable"`). It lives in `execution`, as `ErrCatalogConfiguration` does, because `execution` cannot import `pkg/internal`. `outbound` returns it.
- Sanitizing happens in the egress `DialContext`:
  - a policy refusal returns the bare sentinel;
  - a `*net.DNSError` also returns the bare sentinel. This drops the resolver address and removes the oracle between an internal name that exists and one that doesn't;
  - other dial and TLS errors reach a permitted address and pass through unchanged, so timeouts still classify as timeout.
- `http.Client` wraps the error in a `url.Error` with the request URL, and providers wrap that again. So the stack builder wraps each egress candidate in a model that replaces any error matching the sentinel with the bare sentinel, before `execution.Translate`. The overview's failed attempt then carries only the fixed message, with no host or URL. It is not an `APICallError`, so the fallback decider still falls back.
- Bounded reporting: the 424 document, the overview attempt and every log line on refusal carry no endpoint URL, host or address. They may name the field path and provider name, which is a DNS label. Stack-catalog calls write no physical attempt records, and the model logger logs only bounded status values.
- `ClassifyProvider` and `ClassifyStreamPart` check `errors.Is(err, ErrEgressRefused)` before the `APICallError` status mapping, and return `CategoryEgressRefused`.
- ProviderWire maps it to a fixed document: 424, `failed_dependency`/`failed_dependency`, "provider endpoint refused by the egress policy or not resolvable", not retryable. It uses the same registered status, type and code as the catalog configuration document, with a different message.
- Fallback is unchanged: providers mark transport errors retryable, so a refused candidate falls back. The final error decides the document.

Alternatives considered:

- **Reuse the catalog configuration category:** no new category or document, but its message ("invalid or missing a provider key") misleads, and an operator debugging a refusal can't tell the two apart.
- **Upstream (502):** retryable, which is wrong for a policy refusal.

### Discovery and routing slug

`stackIndex.resolve` and `StackCatalogLister` derive the provider slug as `newCandidate` does: `providerName` for `openai-compatible` when set, otherwise the type. A small shared helper replaces the duplicated logic in `newCandidate`.

## Test plan

No Go test needs external network. Tests that resolve names use a `*net.Resolver` with `PreferGo: true` and an in-memory `Dial`, which answers DNS from a table through `golang.org/x/net/dns/dnsmessage`. The dependency moves from indirect to direct in `ai-gateway/go.mod`. The resolver is injected through an unexported constructor parameter in white-box tests. TLS servers are `httptest.NewTLSServer` on `127.0.0.1`, trusted through an unexported root pool parameter.

| Acceptance | Evidence |
|---|---|
| Public `https` URL works | `classify` table: public IPv4 and IPv6, NAT64 of a public IPv4. Egress client against an `httptest` TLS server on `127.0.0.1` with allow `127.0.0.1`, through the resolver stub (`llm.test` → `127.0.0.1`): unary request succeeds. `stackcatalog` table: `https` `baseURL` on all three types, compatible fields. `service`: the builder uses the egress client only when `baseURL` is set, and passes `providerName` and structured outputs. `gateway` harness: a catalog with `openai-compatible` and `anthropic` custom endpoints, through the test host with the allow setting, reaches the fake server with the catalog key, unary and stream. |
| `http` fails without a request | Admission and `Parse`: `http`, `ftp`, userinfo, query, fragment. Egress `RoundTrip` with an `http` URL: sentinel, zero dials (counting `DialContext`). |
| Refused ranges fail without a request | `classify` table: first and last address of every refused range, `::`, `::1`, `fe80::1%eth0`, `fd00:ec2::254`, `ff02::1`, `::ffff:127.0.0.1`, `::ffff:169.254.169.254`, `64:ff9b::a00:1`, `64:ff9b:1::1`, `::127.0.0.1`, `2001:db8::1`, `2002:a00:1::`. Dial tests with the resolver stub for `127.0.0.1`, `10.0.0.1` and `::1`: sentinel. `Control` refuses before `connect()`, so no packet leaves, including for unroutable addresses. A listener on `127.0.0.1` without an allow entry gets zero accepts. |
| Rebinding | Resolver stub answers `127.0.0.1` for the first lookup and `10.0.0.1`, then `::ffff:127.0.0.1`, for later ones. With allow `127.0.0.1/32`: the first request succeeds, then `CloseIdleConnections`, and later requests fail with the sentinel. The server counts exactly one request. Mixed records (`10.0.0.1`, `127.0.0.1` with allow `127.0.0.1`): one connection, to the allowed address only. |
| Numeric hosts | Static: `127.1`, `2130706433`, `0x7f000001`, `017700000001` refused at admission. Dial: the stub maps `2130706433` to `127.0.0.1`, which is refused at `Control`. |
| IPv6 | All of the above for IPv6 literals and AAAA answers. `[::ffff:10.0.0.1]` literal refused statically. No IPv6 socket is needed: refusals happen before `connect()`. |
| Redirects | A permitted TLS server answers `307` to a second loopback listener (and to `https://169.254.169.254/`). The second listener gets zero requests, and the provider reports the status. |
| Proxy | The egress transport's `Proxy` is nil, and the shared transport's is unchanged. Proxy variables are cached once per process, so behavior is proven end to end, where `HTTPS_PROXY` is set and custom-endpoint calls still go direct. |
| Pool isolation | The operator client and the egress client have distinct transports. Harness: the operator catalog reaches a listener on `127.0.0.1`, then a stack catalog targeting the same listener without an allow entry is refused, and the listener counts only the operator requests. |
| Error text | The refusal and DNS-failure errors, and the overview of a refused-then-fallback call, contain no IPv4 or IPv6 literal (regex), no resolver address, and no list entry. Classification of the wrapped error through `openai-compatible`'s `transportError`, and through the Anthropic and OpenAI SDK error paths, is `CategoryEgressRefused`. ProviderWire unary and stream-setup document bytes. Client matrices already cover 424 `failed_dependency`. `test/providerwire-v4`: both clients receive the document for a catalog whose endpoint is `https://127.0.0.1:<port>`, with zero requests at the listener. |
| Allow and deny lists | `ParseEgressPolicy` table: every entry form, invalid forms (`10.0.0.5/16`, `::ffff:10.0.0.0/104`, `*.`, empty items, duplicates, 257 entries, non-ASCII), error text without the value. Precedence table: deny over allow, unspecified and multicast over allow, allow over built-in, built-in over public, deny over public, wildcard subdomain versus apex, case and trailing dot. Admission with a policy: a deny-listed host is refused, and an allow-listed private literal is accepted. Settings tests for both keys. |
| Operator catalog unaffected | Existing operator tests with `http` loopback endpoints pass unchanged. A harness test with a restrictive deny policy shows the operator catalog still reaching `http://127.0.0.1`. `validateProviderEndpoints` tests unchanged. BYOK tests unchanged. |
| Discovery and routing slug | `StackCatalogLister` and `only`/`order` with a compatible `providerName`. |

Regression: `mise run fmt-check`, `vet`, `lint`, `build`, `test-ai-gateway`, `test-providerwire-v4`, `test-integration`, `verify-ai-gateway-boundary`, `lint-docs`; `openspec validate --strict`. No local e2e; CI runs Playwright.

## End-to-end approach

The e2e fake provider has a private Docker address, which the production policy refuses. The e2e environment adds an allow entry for its hostname through the operator setting. The production default doesn't change, and the entry exists only in `docker-compose.e2e.yaml`.

- `scripts/e2e.sh` adds SANs `DNS:fake-anthropic,DNS:egress-refused,DNS:egress-denied` to the run-time certificate, which Grafana already trusts through `SSL_CERT_FILE`.
- `fake-anthropic` also listens with TLS on 8443, using the mounted certificate. It gets network aliases `egress-refused` and `egress-denied`, so all three names resolve to the same private address. It counts requests per `Host` and reports the counts in its reply text.
- The Grafana service sets:
  - `GF_PLUGIN_GRAFANA_AIGATEWAY_APP_STACK_CATALOG_EGRESS_ALLOW: fake-anthropic,egress-denied`;
  - `GF_PLUGIN_GRAFANA_AIGATEWAY_APP_STACK_CATALOG_EGRESS_DENY: egress-denied`.

  `HTTPS_PROXY` stays set, which also exercises the egress client ignoring it.
- Specs:

| Acceptance | e2e |
|---|---|
| Public `https` works | `anthropic` provider, `baseURL: https://fake-anthropic:8443`: unary and streamed calls succeed, and the reply names the catalog key's hash prefix. The allow entry stands in for "public". Public-address classification is proven in Go. |
| Refused without a request | `https://egress-refused:8443`, the same address without an allow entry: 424 egress document. A following call to `fake-anthropic` reports zero requests for that host. Admission refuses `http://fake-anthropic:8080`, `https://127.0.0.1`, `https://169.254.169.254`, `https://[::ffff:10.0.0.1]` and `https://2130706433` with `400`. |
| Lists apply | `egress-denied` is in both lists: 424 (deny wins). `fake-anthropic` is allowed while `egress-refused` is not, at the same address. |
| Operator catalog unaffected | `routes.spec.ts` unchanged: the operator `fake` provider at `http://fake-anthropic:8080` keeps working. Native stack-catalog calls through the proxy (`catalog-routes.spec.ts`) unchanged. |
| Compatible accepted | Create an `openai-compatible` catalog with `providerName` and `supportsStructuredOutputs`: `201`. Its invocation is proven by the Go harness, because the fake doesn't speak Chat Completions. |

- `catalog.spec.ts`'s "baseURL and name references are refused" test becomes "plain-HTTP baseURL and name references are refused".
- The refusal tests assert that the response bodies contain no IPv4 or IPv6 literal.

## Risks / Trade-offs

- [Proxy-only egress environments can't reach custom endpoints] → documented. A trusted egress proxy can be added later without changing the policy model.
- [An allow hostname entry trusts DNS] → documented, and prefix entries are recommended. Deny prefixes still apply to its addresses.
- [SDK error wrapping hides the sentinel] → tests check `errors.Is` through the Anthropic SDK, the OpenAI SDK and `openai-compatible` paths, since the candidate wrapper relies on it.
- [Internal services on public addresses, such as an organization's own public ranges] → the operator deny list.
- [Ports are unrestricted, so any port of a permitted host is reachable, and timing distinguishes open from closed ports] → no port policy (YAGNI). Bounded by the attempt bound and Grafana's request limits. The README's security guidance names it and points operators at deny prefixes.
- [Deny hostname entries are bypassable through an IP literal or another name for the same address] → documented as a convenience. Deny prefixes are the boundary.
- [A stricter policy makes stored catalogs fail] → catalog configuration at request time for static failures, and the egress document for connect-time ones. A policy change needs a plugin restart.
- [Allow-listed private TLS endpoints need a trusted certificate] → `SSL_CERT_FILE` or system roots, documented.
- [A DNS failure reported as a refusal confuses a typo] → the fixed message says "or not resolvable".
- [#759 catalog UI may only offer `anthropic` and `openai`] → coordinate. This change makes no frontend edits.
- [#762 merge] → appended `Category`, `safeCategories` and `publicErrors` entries, and a `StackCatalogOptions` field. Text conflicts only.

## Follow-ups for #759

- The catalog form needs `baseURL`, the `openai-compatible` type, `providerName` and `supportsStructuredOutputs` once both changes land. #759 records the same follow-up.

## Migration Plan

Additive. With both settings unset, the built-in policy applies only to the new custom endpoints. Existing stack catalogs have no `baseURL` and are unaffected. Rolling back makes admission refuse `baseURL` again. Stored catalogs with custom endpoints then fail per request with the catalog configuration document.

## Decisions recorded at review

1. **Proxy:** direct connections only for custom endpoints. A trusted egress proxy can come later.
2. **Error mapping:** a new `egress refused` category and 424 document.
3. **DNS failures:** the same refusal error and document, so there is no internal-name oracle.
4. **Allow semantics:** exceptions to the built-in refusal, not an exclusive list.
5. **Scope:** only providers with `baseURL`; native endpoints keep the shared client and its proxy.
6. **Hostname entries:** allowed in both lists. Deny hostnames are a convenience; deny prefixes are the boundary.
7. **Never-dialable:** allow entries can lift loopback and private ranges, never unspecified or multicast addresses.
8. **Ports:** unrestricted, documented next to the port-scanning risk.
