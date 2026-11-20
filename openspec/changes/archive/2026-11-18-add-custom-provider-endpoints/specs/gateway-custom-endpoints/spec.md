# Spec Delta

## Purpose

Lets customers point stack-catalog providers and request accounts at provider endpoints other than the native ones, but only at endpoints the operator lists, so a customer can't make the Gateway connect to an arbitrary destination.

## ADDED Requirements

### Requirement: Custom endpoint list setting
The `custom_endpoints` setting SHALL accept comma-separated base URLs, each absolute, `http` or `https`, with an ASCII host and no userinfo, query, fragment, wildcard, backslash, or path segment that decodes to `.`, `..` or a value containing `/`, `\`, `%` or `;`. An unset or empty setting SHALL allow no custom endpoint. Empty, duplicate after normalization or invalid entries SHALL fail startup with a reason naming the setting, without echoing the value.

#### Scenario: Valid list
- **WHEN** the setting is `https://llm.corp.example/v1, http://10.20.0.5:8080`
- **THEN** the Gateway SHALL start with both entries

#### Scenario: Invalid entry
- **WHEN** an entry is `https://user:secret@llm.example`, `https://*.example.com`, `https://llm.example/v1?x=1`, an empty item, or a duplicate such as `https://LLM.example/v1/` next to `https://llm.example/v1`
- **THEN** startup SHALL fail with a reason naming the setting, and the reason SHALL NOT contain the entry

#### Scenario: No list
- **WHEN** the setting is unset
- **THEN** every custom endpoint SHALL be refused, and native endpoints SHALL keep working

### Requirement: Custom endpoint matching
A custom endpoint SHALL be allowed when it matches an entry. Its scheme and host SHALL equal the entry's, case-insensitively. Its port SHALL equal the entry's as written, so an omitted port matches only an omitted one. Its path, ignoring one trailing `/`, SHALL equal the entry's path or continue it after a `/`. Matching SHALL be a string comparison with no DNS lookup or connection.

#### Scenario: Path prefix on a segment boundary
- **WHEN** the list holds `https://llm.corp.example/v1`
- **THEN** `https://llm.corp.example/v1`, `https://LLM.corp.example/v1/` and `https://llm.corp.example/v1/team` SHALL match, and `https://llm.corp.example/v10`, `https://llm.corp.example/` and `https://llm.corp.example` SHALL NOT

#### Scenario: Scheme and port are exact
- **WHEN** the list holds `https://llm.corp.example`
- **THEN** `http://llm.corp.example`, `https://llm.corp.example:443` and `https://llm.corp.example:8443` SHALL NOT match, and `https://llm.corp.example/any/path` SHALL match

#### Scenario: Plain HTTP only when listed
- **WHEN** the list holds `http://10.20.0.5:8080`
- **THEN** `http://10.20.0.5:8080/v1` SHALL match, and `https://10.20.0.5:8080` SHALL NOT

#### Scenario: Other names for a listed address
- **WHEN** the list holds `https://llm.corp.example` and an endpoint uses another name or the IP address of the same host
- **THEN** the endpoint SHALL NOT match

#### Scenario: Dot segments and encoded separators
- **WHEN** an endpoint's path holds a backslash or a segment that decodes to `.`, `..` or a value containing `/`, `\`, `%` or `;`, such as `https://llm.corp.example/v1/../admin`, `.../v1/..%2fadmin`, `.../v1/..%5cadmin`, `.../v1/%252e%252e/admin` or `.../v1/..;/admin`
- **THEN** it SHALL be refused as malformed, whatever the list holds, because the provider SDKs decode the path and resolve dot segments when they join their own path, and some servers strip `;` path parameters before resolving them

#### Scenario: Non-ASCII host
- **WHEN** an endpoint's host holds a non-ASCII character, even one that lowercases to a listed ASCII host
- **THEN** it SHALL be refused as malformed

#### Scenario: Accepted endpoint is the destination
- **WHEN** the list accepts an endpoint for an `anthropic`, `openai` or `openai-compatible` provider
- **THEN** the provider SHALL send its request to the entry's scheme, host and port, under the entry's path

### Requirement: Native default counts as unset
A stack-catalog provider's or request account's `baseURL` that equals its provider type's native endpoint, with scheme and host compared case-insensitively and one trailing `/` ignored, SHALL count as unset. It SHALL need no entry, and any other form, such as an added port or path, SHALL be a custom endpoint.

#### Scenario: Explicit native endpoint without a list
- **WHEN** the setting is unset and an `openai` provider or account sets `baseURL: https://API.openai.com/v1/`
- **THEN** it SHALL be accepted and SHALL reach the native endpoint, as without `baseURL`

#### Scenario: Native host with another path
- **WHEN** an `openai` account sets `baseURL: https://api.openai.com/v2` and the list doesn't hold it
- **THEN** it SHALL be refused as an unlisted custom endpoint

### Requirement: Where the list is checked
The list SHALL be checked at stack-catalog admission, on every stack-catalog request and when BYOK controls are decoded, for every provider or account with a custom `baseURL`, including unused BYOK providers. A refusal SHALL name at most the field path, never the URL. Admission SHALL refuse with 400, a stack-catalog request SHALL fail with the catalog configuration category, and BYOK decoding SHALL fail with the BYOK credentials category, each before any provider request.

#### Scenario: Unlisted endpoint at admission
- **WHEN** a writer saves a catalog provider with an unlisted `baseURL`
- **THEN** the write SHALL be refused with 400 and a reason naming `providers.<name>.baseURL`, without the URL

#### Scenario: Entry removed after the catalog was saved
- **WHEN** a stored catalog's endpoint was listed when saved and the operator removes its entry and restarts the Gateway
- **THEN** every call to that catalog's models SHALL fail with the catalog configuration document, with zero provider requests

#### Scenario: Unlisted request account endpoint
- **WHEN** a BYOK call's account, used or not, sets an unlisted custom `baseURL`
- **THEN** the call SHALL fail with the BYOK credentials document, without the URL and with zero provider requests

### Requirement: Connections to custom endpoints
A listed custom endpoint SHALL be reached through the shared model HTTP client, with its proxy settings, response bounds and redirect refusal. The operator catalog SHALL keep its own endpoint rules and SHALL NOT be checked against the list.

#### Scenario: Listed endpoint
- **WHEN** a stack-catalog provider or request account uses a listed `baseURL`
- **THEN** its requests SHALL go to that endpoint with only its own key, through the shared model client

#### Scenario: Operator catalog endpoint
- **WHEN** the operator catalog has a provider with `baseURL: http://10.0.0.5:8080` and the list is empty
- **THEN** operator-catalog calls SHALL keep reaching it

### Requirement: Custom endpoint documentation
`ai-gateway/README.md` SHALL describe the setting, its format and matching rules, the errors at each check, and that a listed endpoint is trusted: operators SHALL be told to list only endpoints they control or vendors whose DNS they trust, because whoever controls a listed hostname's DNS controls the destination, and that in `cloud` mode any user or service account can use a listed endpoint through BYOK.

#### Scenario: Operator reads the guidance
- **WHEN** an operator reads the custom endpoints section and the settings table
- **THEN** they SHALL find each of these points

### Requirement: Custom endpoint evidence
Each rule SHALL have Go tests that need no external network. End-to-end tests SHALL prove a listed endpoint works for a stack catalog, unlisted endpoints are refused at admission, and the operator catalog's plain-HTTP endpoint still works. Request account endpoints SHALL be proven through the built plugin in `cloud` mode with both registered clients.

#### Scenario: End-to-end listed endpoint
- **WHEN** the e2e suite runs with an e2e-only entry for the fake provider's HTTPS listener
- **THEN** a stack-catalog call to it SHALL succeed, and saving an endpoint outside the entry SHALL be refused
