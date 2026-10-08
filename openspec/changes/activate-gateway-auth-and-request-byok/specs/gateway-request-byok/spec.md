## ADDED Requirements

### Requirement: Authenticated request-only destination
Authenticated BYOK inference SHALL require a provider/model selector in the existing ai-language-model-id header. The selector SHALL be valid UTF-8, have a supported case-sensitive provider prefix before the first slash and a nonempty native-model suffix. The service SHALL reuse the request-only engine's decoder and the existing whole-request and HTTP header bounds without adding selector grammar or a separate byte ceiling. The suffix SHALL be preserved, including additional slashes, as model data rather than a URL. Initial providers SHALL be anthropic and openai, using the native Messages and Responses adapters respectively.

The selector SHALL NOT resolve configured models, aliases, provider instances, endpoints, defaults or fallback routes. Advisory native model lists SHALL NOT act as a configured-catalog allowlist. Unknown native models SHALL be decided by the selected native provider without unsolicited inventory or validation calls.

#### Scenario: Native model absent from configured catalog
- **WHEN** a valid anthropic/model-name selector and Anthropic request credentials are supplied
- **THEN** the adapter SHALL receive model-name without consulting any catalog, regardless of whether it is configured internally

#### Scenario: Flat configured alias is supplied
- **WHEN** a BYOK caller names assistant or another selector without a provider/model split
- **THEN** selection SHALL fail before I/O rather than expand a configured alias

#### Scenario: Selector boundaries
- **WHEN** a selector has an empty provider/model component, invalid UTF-8 or an unsupported provider
- **THEN** the request SHALL fail with a bounded capability/field diagnostic before provider construction

### Requirement: Request-only native consumption guards
BYOK invocation SHALL retain gateway-native-provider-options protections at the
actual consuming namespace and scope without restoring catalog inventories.
Invalid consumed Anthropic MCP configuration/history, container skills and
provider-side fallback controls SHALL fail before native I/O in both invocation
modes. Valid bounded MCP configuration and configured provider-executed history
SHALL remain supported. Harmless ignored
fields and unrelated namespaces SHALL NOT be rejected by blanket spelling rules.

#### Scenario: Consumed execution bypass is refused
- **WHEN** an Anthropic BYOK request supplies invalid consumed MCP configuration/history, a container skill or a provider-side fallback control
- **THEN** it SHALL return the fixed invalid-request error without any credential attempt

#### Scenario: Ordinary native fields survive
- **WHEN** native options carry a safe container ID or ignored protected-looking fields in non-consuming scopes
- **THEN** the request SHALL reach the adapter without changing its selected model, account or destination

#### Scenario: Valid MCP configuration and history survive
- **WHEN** an Anthropic BYOK request supplies valid bounded MCP servers and corresponding provider-executed history
- **THEN** the native adapter SHALL receive the MCP configuration and history without consulting configured accounts

### Requirement: BYOK credentials stay out of automatic server surfaces
The host SHALL exclude the complete BYOK subtree and actual authentication credentials from server logs, metrics, Agent Observability/Sigil, response/error diagnostics, routing metadata and recorded provider fixtures. Native rejection echoes and authentication transport diagnostics SHALL not reflect selected credentials. Redaction SHALL be source-specific, not based on censoring key-looking application text. Supported provider/model identity, application content and native response values SHALL remain available under their existing contracts.

Diagnostic messages SHALL use bounded approved capability names/schema paths and bounded indices/counts, never arbitrary rejected key names, values or serialized bodies. Detailed public attempt evidence SHALL remain owned by its separate contract; this capability SHALL NOT introduce a second metadata/error protocol.

#### Scenario: Provider echoes the rejected key
- **WHEN** a fake provider rejection includes the selected dummy API key in authentication-related error material
- **THEN** automatic server capture and returned diagnostics SHALL omit that credential while preserving safe status/provider/attempt facts

#### Scenario: Ordinary application text resembles a key
- **WHEN** supported prompt/output text contains a key-looking string unrelated to authentication material
- **THEN** the response SHALL preserve that content rather than apply token-pattern censorship

### Requirement: Evidence remains independent and provenance-correct
Acceptance SHALL include exact-pinned Vercel HTTP captures, independent Go-client tests without AGPL imports, fake native HTTP requests, real unified-command tests, middleware sink tests and race/lifetime tests. Synthetic credential cases SHALL use dummy values in focused tests, not invented recorded/upstream conformance inputs. Existing authentic provider inputs SHALL remain unchanged. Applicable parity/module/frontend checks SHALL run, with coverage gaps and deferred families/routing explicitly recorded.

#### Scenario: Catalog is inaccessible to BYOK
- **WHEN** a spy catalog that fails on every call accompanies BYOK inference and discovery tests
- **THEN** inference SHALL work from request data and discovery SHALL be explicitly rejected, with zero catalog calls in both cases

#### Scenario: Provider behavior is claimed
- **WHEN** evidence is summarized
- **THEN** fake HTTP and registered-client evidence SHALL be distinguished from live native acceptance and Vercel private-service behavior
