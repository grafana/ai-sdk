## MODIFIED Requirements

### Requirement: HTTPS only
A custom endpoint of a stack catalog or a request account SHALL use the `https` scheme. Stack-catalog admission, every stack-catalog request and BYOK decoding SHALL refuse any other scheme, and the egress client SHALL send no request whose URL is not `https`. No setting SHALL permit plain HTTP.

#### Scenario: Plain HTTP endpoint
- **WHEN** a writer saves a catalog provider with `baseURL: http://llm.example.com`
- **THEN** the write SHALL be refused, and no key SHALL be sent anywhere

#### Scenario: Plain HTTP request account endpoint
- **WHEN** a BYOK account sets `baseURL: http://llm.example.com`
- **THEN** decoding SHALL fail with the BYOK credentials category, and no key SHALL be sent anywhere

#### Scenario: Plain HTTP reaches the client
- **WHEN** a request URL with the `http` scheme reaches the egress client
- **THEN** it SHALL fail with the egress refusal, with zero connections

### Requirement: Egress policy scope
The egress policy SHALL govern every connection for a custom endpoint: a stack-catalog provider's or request account's `baseURL` other than its type's native default. It SHALL NOT govern the operator catalog, or stack-catalog providers and request accounts without one, which use their type's native endpoint through the shared model HTTP client.

#### Scenario: Custom stack-catalog endpoint
- **WHEN** a stack-catalog call selects a provider with a custom `baseURL`
- **THEN** its connections SHALL be checked by the egress policy

#### Scenario: Native endpoint in a stack catalog
- **WHEN** a stack-catalog call selects an `anthropic` provider without `baseURL`
- **THEN** it SHALL reach the native endpoint through the shared model HTTP client, as before

#### Scenario: Operator catalog with a private endpoint
- **WHEN** the operator catalog has a provider with `baseURL: http://10.0.0.5:8080` and a stack catalog has a custom endpoint
- **THEN** operator-catalog calls SHALL keep reaching `http://10.0.0.5:8080`, and startup validation of operator endpoints SHALL be unchanged

#### Scenario: Explicit native endpoint
- **WHEN** a stack-catalog provider or a request account sets a `baseURL` equal, after normalization, to its type's native default
- **THEN** it SHALL count as unset: it SHALL use the shared model HTTP client and its proxy settings, and the egress lists SHALL NOT apply to it

#### Scenario: Custom request account endpoint
- **WHEN** a BYOK call's account sets a custom `baseURL`
- **THEN** its connections SHALL be checked by the egress policy, as for a stack-catalog provider

### Requirement: Static endpoint checks
Stack-catalog admission, every stack-catalog request and BYOK decoding SHALL refuse a custom endpoint whose host is not an IP literal or an ASCII DNS name whose last label starts with a letter, whose IP literal the policy refuses, or whose host matches a deny entry. They SHALL make no DNS lookup. Stack-catalog request-time failures SHALL use the catalog configuration category, and BYOK failures the BYOK credentials category.

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

#### Scenario: Request account with a refused literal
- **WHEN** a BYOK account sets `baseURL: https://127.0.0.1`
- **THEN** decoding SHALL fail with the BYOK credentials category, with zero provider requests

### Requirement: Operator allow and deny lists
The settings `custom_endpoint_allow` and `custom_endpoint_deny` SHALL each accept comma-separated entries: an IP address, a canonical CIDR prefix, a hostname, or `*.` and a domain, which matches its subdomains only. Hostnames SHALL match case-insensitively, without a trailing dot. Empty, duplicate, malformed or non-canonical entries SHALL fail startup without echoing the value.

#### Scenario: Valid lists
- **WHEN** the allow list is `10.20.0.0/16,llm.corp.example` and the deny list is `*.internal.example,203.0.113.7`
- **THEN** the Gateway SHALL start with those entries

#### Scenario: Invalid entry
- **WHEN** an entry is `10.20.0.5/16`, `::ffff:10.0.0.0/104`, an empty item or `*.`
- **THEN** startup SHALL fail with a reason naming the setting

### Requirement: Custom endpoint documentation
`ai-gateway/README.md` SHALL describe custom endpoints of stack catalogs and request accounts: HTTPS only, the refused ranges, connect-time checks, that proxies are not used, the two settings with their format and precedence, the egress refused error, that hostname allow entries trust that name's DNS, that deny hostname entries are a convenience and prefixes the boundary, and that ports are unrestricted, so deny prefixes are how operators stop port scanning.

#### Scenario: Operator reads the guidance
- **WHEN** an operator reads the custom endpoint section and the settings table
- **THEN** they SHALL find each of these points

### Requirement: Egress evidence
Each policy requirement SHALL have Go tests that need no external network. End-to-end tests SHALL prove a stack-catalog custom endpoint works through an operator allow entry, a refused and a denied endpoint fail, and the operator catalog's private plain-HTTP endpoint still works. Request account endpoints SHALL be proven through the built plugin in `cloud` mode with both registered clients.

#### Scenario: End-to-end custom endpoint
- **WHEN** the e2e suite runs with an e2e-only allow entry for the fake provider's hostname
- **THEN** a stack-catalog call to its HTTPS listener SHALL succeed, and calls to other names for the same address SHALL fail with the egress refused document

#### Scenario: Request account endpoints through the clients
- **WHEN** the plugin-protocol suite sends BYOK accounts with a refused literal and a refused loopback name through both clients
- **THEN** each client SHALL receive the BYOK credentials document and the egress refused document respectively, and the endpoints SHALL receive no connection
