# Proposal

## Why

Stack catalogs can only reach the native `anthropic` and `openai` endpoints. Request accounts (BYOK) accept only the exact native URL: the approved-destination list they define is never set in production. Customers need regional endpoints, hosted proxies and `openai-compatible` backends. The request leaves from the Gateway, so the customer must not choose an arbitrary destination. The operator decides which endpoints customers may use, by listing them. A listed endpoint is trusted, so the Gateway needs no network policy of its own when it connects.

## What Changes

- **One operator setting, `custom_endpoints`:** comma-separated base URLs that customers may use as a provider `baseURL`. Unset or empty means no custom endpoints.
- **Matching is a string comparison** with no DNS lookup and no connection check.
  - The scheme, host and port must equal an entry's. Scheme and host compare case-insensitively, and the port as written.
  - The path must equal the entry's path or continue it on a `/` boundary, ignoring one trailing `/`.
  - The `http` scheme is accepted only where an entry lists it. There are no wildcards.
- **A native default counts as unset.** A `baseURL` equal to its provider type's native endpoint needs no entry.
- **Where the list is checked:**
  - **Stack-catalog admission:** an unlisted custom `baseURL` is refused with a 400 that names the field path.
  - **Every stack-catalog request:** an entry removed from the list takes effect on the next call, as the existing catalog configuration error (424).
  - **BYOK decoding:** an unlisted custom `baseURL` on any supplied account gets the existing fixed BYOK credentials error (400).
- **Stack catalogs** accept `baseURL`, and `openai-compatible` with `providerName` and `supportsStructuredOutputs`, under the operator catalog's field rules. Stack discovery and routing use the operator provider-slug rule.
- **Connections** use the shared model client, with its proxy settings, response bounds and redirect refusal, as every other model does. The URL form checks stay: absolute, a host, and no userinfo, query or fragment.
- **Removed:** the BYOK service-approved destination requirement and its unused parameter.
- **Docs:** README guidance on custom endpoints and their security, the settings table, the BYOK client guide, the glossary and the architecture walkthrough.

## Capabilities

### New Capabilities

- `gateway-custom-endpoints`: the operator list of custom provider endpoints, the matching rules, where they apply, their errors, the connection path, and their documentation and evidence.

### Modified Capabilities

- `gateway-model-catalog-kind`: stack catalogs accept `baseURL` and `openai-compatible` with its fields, under the list, instead of refusing them.
- `gateway-stack-catalog-runtime`: every request checks custom endpoints against the list, and provider slugs follow the operator rule.
- `gateway-configured-discovery`: catalog route discovery reports the same provider slug.
- `gateway-request-byok`: the account endpoint rule follows the list; the service-approved destination requirement is removed.
- `gateway-provider-configuration`: stack-catalog and request-account keys may go to a listed `baseURL` through the shared model client.

## Impact

- **Code:**
  - `ai-gateway/pkg/internal/outbound`: `ValidateEndpoint` stays, plus a new endpoint list with its matching;
  - `ai-gateway/pkg/internal/config`: the setting;
  - `ai-gateway/pkg/internal/stackcatalog`: policy, conversion and the validator, which takes the list;
  - `ai-gateway/pkg/internal/byok`: decoding takes the list;
  - `ai-gateway/pkg/internal/service`: stack candidates with their endpoint and fields, the provider slug, and the list for stack catalogs and BYOK;
  - `ai-gateway/pkg/internal/gateway`: wiring.
- **APIs:**
  - No schema change: the fields already exist in the `ModelCatalog` kind.
  - No new error document, category or status.
  - One new plugin setting.
- **Tests:**
  - Go unit and harness tests, with no external network.
  - `test/providerwire-v4` scenarios through both registered clients.
  - Playwright specs with an e2e-only list entry for the fake provider.
- **Not parity-sensitive:** the Vercel AI SDK has no tenant catalog. The pinned `@ai-sdk/gateway` 4.0.132 types `byok` as opaque, service-validated records. No stream, chunk, provider request or wire format changes.
