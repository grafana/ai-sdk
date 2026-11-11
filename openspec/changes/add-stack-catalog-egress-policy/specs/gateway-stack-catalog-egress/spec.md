# Spec Delta

## Purpose

Keeps a customer-chosen stack-catalog provider endpoint from reaching internal or non-public destinations from the Gateway, while letting stacks use public HTTPS endpoints and operators adjust the policy.

## ADDED Requirements

### Requirement: Egress policy scope
The egress policy SHALL govern every connection made for a stack-catalog provider that sets `baseURL`. It SHALL NOT govern the operator catalog, BYOK accounts, or stack-catalog providers without `baseURL`, which use their type's native endpoint through the shared model HTTP client.

#### Scenario: Custom stack-catalog endpoint
- **WHEN** a stack-catalog call selects a provider with `baseURL`
- **THEN** its connections SHALL be checked by the egress policy

#### Scenario: Native endpoint in a stack catalog
- **WHEN** a stack-catalog call selects an `anthropic` provider without `baseURL`
- **THEN** it SHALL reach the native endpoint through the shared model HTTP client, as before

#### Scenario: Operator catalog with a private endpoint
- **WHEN** the operator catalog has a provider with `baseURL: http://10.0.0.5:8080` and a stack catalog has a custom endpoint
- **THEN** operator-catalog calls SHALL keep reaching `http://10.0.0.5:8080`, and startup validation of operator endpoints SHALL be unchanged

### Requirement: HTTPS only
A custom stack-catalog endpoint SHALL use the `https` scheme. Admission and every request SHALL refuse any other scheme, and the egress client SHALL send no request whose URL is not `https`. No setting SHALL permit plain HTTP.

#### Scenario: Plain HTTP endpoint
- **WHEN** a writer saves a catalog provider with `baseURL: http://llm.example.com`
- **THEN** the write SHALL be refused, and no key SHALL be sent anywhere

#### Scenario: Plain HTTP reaches the client
- **WHEN** a request URL with the `http` scheme reaches the egress client
- **THEN** it SHALL fail with the egress refusal, with zero connections

### Requirement: Refused destination ranges
Unless an allow entry lifts the refusal, the policy SHALL refuse every address that is not globally reachable unicast: IPv4 `0/8`, `10/8`, `100.64/10`, `127/8`, `169.254/16`, `172.16/12`, `192.0.0/24`, `192.0.2/24`, `192.88.99/24`, `192.168/16`, `198.18/15`, `198.51.100/24`, `203.0.113/24`, `224/4`, `240/4`; IPv6 outside `2000::/3` other than embedded IPv4 forms, and `2001::/23`, `2001:db8::/32`, `2002::/16`, `3fff::/20` within it.

#### Scenario: Every refused range
- **WHEN** an endpoint resolves to the first or last address of any listed range
- **THEN** the connection SHALL be refused

#### Scenario: Public addresses
- **WHEN** an endpoint resolves only to globally reachable unicast addresses, such as `8.8.8.8` or `2606:4700::1111`
- **THEN** the policy SHALL permit the connection

#### Scenario: IPv6 loopback, link-local, unique local and multicast
- **WHEN** an endpoint resolves to `::1`, `::`, `fe80::1`, `fe80::1%eth0`, `fd00:ec2::254` or `ff02::1`
- **THEN** the connection SHALL be refused

### Requirement: Embedded IPv4 addresses
An IPv4-mapped IPv6 address (`::ffff:0:0/96`) and a NAT64 well-known-prefix address (`64:ff9b::/96`) SHALL be checked as the IPv4 address they embed. The NAT64 local-use prefix `64:ff9b:1::/48` SHALL be refused.

#### Scenario: IPv4-mapped loopback
- **WHEN** an endpoint is `https://[::ffff:127.0.0.1]` or resolves to `::ffff:169.254.169.254`
- **THEN** the connection SHALL be refused

#### Scenario: NAT64 to a public address
- **WHEN** an endpoint resolves to `64:ff9b::808:808`
- **THEN** the policy SHALL permit it as `8.8.8.8`

#### Scenario: NAT64 to a private address
- **WHEN** an endpoint resolves to `64:ff9b::a00:1`
- **THEN** the connection SHALL be refused as `10.0.0.1`

### Requirement: Connect-time enforcement
The policy SHALL be enforced on every address the egress client connects to, after DNS resolution and before the connection is opened, for every connection attempt, including each address family tried. A refused address SHALL receive no packet, and the decision SHALL NOT depend on an earlier resolution.

#### Scenario: DNS rebinding
- **WHEN** a host resolves to a permitted address for one connection and to `10.0.0.1` for the next
- **THEN** the second connection SHALL be refused, and the refused address SHALL receive no request

#### Scenario: Mixed records
- **WHEN** a host resolves to both a refused and a permitted address
- **THEN** no connection SHALL be opened to the refused address

#### Scenario: Numeric host forms
- **WHEN** a request reaches a host that the system resolver reads as a loopback address, such as `2130706433`
- **THEN** the connection SHALL be refused

### Requirement: No proxy for custom endpoints
The egress client SHALL connect directly to the endpoint and SHALL ignore `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` and any other proxy configuration, so the checked address is always the destination. The shared model HTTP client SHALL keep its proxy behavior.

#### Scenario: Proxy set in the environment
- **WHEN** `HTTPS_PROXY` is set and a stack-catalog call uses a custom endpoint
- **THEN** the proxy SHALL receive no connection, and the endpoint's resolved address SHALL be checked

#### Scenario: Native endpoints with a proxy
- **WHEN** `HTTPS_PROXY` is set and a call reaches a native endpoint through the shared client
- **THEN** the call SHALL use the proxy as before

### Requirement: Separate connection pool
The egress client SHALL keep its own connections and SHALL NOT reuse a connection opened by the shared model HTTP client, or one opened before the policy permitted its address.

#### Scenario: Same host in both catalogs
- **WHEN** the operator catalog and a stack catalog both use `https://llm.internal.example`, which resolves to a private address
- **THEN** operator calls SHALL succeed, and stack-catalog calls SHALL be refused even while an operator connection to that host is idle

### Requirement: Redirects are not followed
The egress client SHALL NOT follow redirects, and SHALL apply the shared client's response-header timeout and response byte bound.

#### Scenario: Redirect to an internal address
- **WHEN** a permitted endpoint answers `307` with `Location: https://169.254.169.254/`
- **THEN** the redirect target SHALL receive no request

### Requirement: Operator allow and deny lists
The settings `stack_catalog_egress_allow` and `stack_catalog_egress_deny` SHALL each accept comma-separated entries: an IP address, a canonical CIDR prefix, a hostname, or `*.` and a domain, which matches its subdomains only. Hostnames SHALL match case-insensitively, without a trailing dot. Empty, duplicate, malformed or non-canonical entries SHALL fail startup without echoing the value.

#### Scenario: Valid lists
- **WHEN** the allow list is `10.20.0.0/16,llm.corp.example` and the deny list is `*.internal.example,203.0.113.7`
- **THEN** the Gateway SHALL start with those entries

#### Scenario: Invalid entry
- **WHEN** an entry is `10.20.0.5/16`, `::ffff:10.0.0.0/104`, an empty item or `*.`
- **THEN** startup SHALL fail with a reason naming the setting

### Requirement: List precedence
A destination SHALL be refused when its host matches a deny hostname entry or a dialed address is in a deny prefix. Otherwise it SHALL be permitted when its host matches an allow hostname entry or the address is in an allow prefix. Otherwise the refused ranges SHALL apply. Unspecified and multicast addresses SHALL be refused even when allowed. A deny hostname entry is a convenience, not a boundary: the same address stays reachable through an IP literal or another name.

#### Scenario: Allow lifts a built-in refusal
- **WHEN** the allow list holds `10.20.0.0/16` and an endpoint resolves to `10.20.1.2`
- **THEN** the connection SHALL be permitted

#### Scenario: Deny wins
- **WHEN** `llm.corp.example` is in both lists
- **THEN** connections to it SHALL be refused

#### Scenario: Deny a public destination
- **WHEN** the deny list holds `203.0.113.7/32` or `api.example.com`, and an endpoint resolves to that address or uses that host
- **THEN** the connection SHALL be refused

#### Scenario: Allow cannot reach unspecified addresses
- **WHEN** the allow list holds `0.0.0.0/0` and an endpoint resolves to `0.0.0.0`
- **THEN** the connection SHALL be refused

#### Scenario: Deny hostname bypassed by another name
- **WHEN** the deny list holds only `api.example.com`, and an endpoint uses another name or the IP literal for the same public address
- **THEN** the connection SHALL be permitted, and only a deny prefix SHALL refuse that address

#### Scenario: Allow is not exclusive
- **WHEN** the allow list holds only `llm.corp.example`
- **THEN** other endpoints resolving to public addresses SHALL still be permitted

### Requirement: Static endpoint checks
Admission and every request SHALL refuse a custom endpoint whose host is not an IP literal or an ASCII DNS name whose last label starts with a letter, whose IP literal the policy refuses, or whose host matches a deny entry. They SHALL make no DNS lookup. Request-time failures SHALL use the catalog configuration category.

#### Scenario: Metadata address literal
- **WHEN** a writer saves `baseURL: https://169.254.169.254/v1`
- **THEN** the write SHALL be refused with a reason naming the field path

#### Scenario: Numeric host form
- **WHEN** a writer saves `baseURL: https://0x7f000001` or `https://127.1`
- **THEN** the write SHALL be refused

#### Scenario: Hostname resolving to a private address
- **WHEN** a writer saves a hostname that resolves to a private address
- **THEN** the write SHALL be accepted, and every call SHALL be refused when connecting

#### Scenario: Policy changed after the catalog was saved
- **WHEN** a stored catalog's endpoint host matches a deny entry added later
- **THEN** every call SHALL fail with the catalog configuration category, with zero provider requests

### Requirement: Secret-safe refusal
A refused connection and a failed DNS resolution of a custom endpoint SHALL fail the attempt with one fixed egress refusal error. Neither it, the error document nor any log line SHALL carry the endpoint URL or host, a resolved or resolver address, or the matched range or entry; they MAY name the field path and provider name. A refused attempt SHALL be fallback-eligible, and a failed call with a refused attempt SHALL fail with the egress refused category.

#### Scenario: Refused primary, permitted fallback
- **WHEN** a model's primary endpoint is refused and its fallback succeeds
- **THEN** the call SHALL succeed, and the overview's failed attempt SHALL carry only the fixed refusal message, without the endpoint host or any address

#### Scenario: Only candidate refused
- **WHEN** the only candidate resolves to `10.0.0.1`
- **THEN** the call SHALL fail with the egress refused document, and no response, log, metric or Agent Observability record SHALL contain `10.0.0.1`

#### Scenario: Refused primary, failing fallback
- **WHEN** a model's primary endpoint is refused and its fallback fails with `503`
- **THEN** the call SHALL fail with the egress refused document, because retrying cannot succeed until the catalog or policy changes

#### Scenario: Client cancels after a refused attempt
- **WHEN** a model's primary endpoint is refused and the caller cancels while the fallback runs
- **THEN** the call SHALL fail with the cancellation category, not egress refused

#### Scenario: Unresolvable host
- **WHEN** a custom endpoint host has no DNS records
- **THEN** the attempt SHALL fail with the same fixed refusal error as a refused address

### Requirement: Connection errors name no address
A connection, TLS handshake or response read of a custom endpoint that fails after the policy permitted it SHALL fail with a fixed connection error, or a fixed timeout error that classifies as timeout. Neither SHALL carry the Gateway's address, a resolved address or certificate names. A cancellation SHALL stay a cancellation.

#### Scenario: Endpoint resets the TLS handshake
- **WHEN** a permitted endpoint resets the connection during the handshake or while streaming
- **THEN** the overview and any stream error SHALL carry the fixed connection error, without the endpoint URL or any IP address

#### Scenario: Certificate for another name
- **WHEN** a permitted endpoint presents a certificate that does not name its host
- **THEN** the attempt SHALL fail with the fixed connection error, without the certificate's names

#### Scenario: Endpoint stalls its response headers
- **WHEN** a permitted endpoint does not send headers within the response-header timeout
- **THEN** the attempt SHALL fail with the fixed timeout error and the timeout category

### Requirement: Custom endpoint documentation
`ai-gateway/README.md` SHALL describe custom stack-catalog endpoints: HTTPS only, the refused ranges, connect-time checks, that proxies are not used, the two settings with their format and precedence, the egress refused error, that hostname allow entries trust that name's DNS, that deny hostname entries are a convenience and prefixes the boundary, and that ports are unrestricted, so deny prefixes are how operators stop port scanning.

#### Scenario: Operator reads the guidance
- **WHEN** an operator reads the stack catalog section and the settings table
- **THEN** they SHALL find each of these points

### Requirement: Egress evidence
Each policy requirement SHALL have Go tests that need no external network. End-to-end tests SHALL prove a custom endpoint works through an operator allow entry, a refused and a denied endpoint fail, and the operator catalog's private plain-HTTP endpoint still works.

#### Scenario: End-to-end custom endpoint
- **WHEN** the e2e suite runs with an e2e-only allow entry for the fake provider's hostname
- **THEN** a stack-catalog call to its HTTPS listener SHALL succeed, and calls to other names for the same address SHALL fail with the egress refused document
