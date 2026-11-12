# Proposal

## Why

Request accounts (BYOK) and stack catalogs both let a customer choose where the Gateway connects, but they follow different rules for it. A stack-catalog provider may set any `https` `baseURL` under the egress policy. It is checked without DNS at admission, connects through the egress client, and gets bare egress errors. A request account's `baseURL` must equal its provider's native endpoint or appear on a service-approved list. Production passes no list, so BYOK is native-only, and the approval parameter is dead code. One concern should have one rule: a request account's `baseURL` should behave exactly like a stack-catalog provider's.

## What Changes

- **BREAKING for request accounts:**
  - An account `baseURL` is now accepted under the egress policy, instead of only the exact native URL. Any `https` endpoint the policy permits works.
  - A refused one fails before any provider request:
    - at decode with the existing BYOK credentials document (400) when the URL form, the scheme, the host or a deny entry refuses it without DNS;
    - when connecting with the egress refused document (424) when the resolved address is refused.
  - An account with a custom `baseURL` connects through the egress client, which uses no proxy. A `baseURL` equal to the native default counts as unset, for request accounts and stack providers alike, so it keeps the shared client and the proxy.
- **One rule, shared code:**
  - the static endpoint check moves from `stackcatalog` to `outbound`;
  - the bare-egress-error wrapper moves from `service` to `outbound`;
  - a two-client value chooses the egress client for every model with a `baseURL`.

  Stack catalogs and BYOK use all three, so the rule can't drift.
- **Removed:** the BYOK service-approved destination list (`approvedBaseURLs`) and its requirement.
- **Renamed settings:** `stack_catalog_egress_allow` and `stack_catalog_egress_deny` become `custom_endpoint_allow` and `custom_endpoint_deny`, because they now govern both sources. The settings haven't been released.
- **Docs:** the README custom-endpoint section and settings table, the client guide's BYOK section, the glossary and the architecture walkthrough.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-request-byok`:
  - the request-destination rule names the egress policy instead of an approval map;
  - a new requirement governs account endpoints;
  - construction uses policy-admitted endpoints;
  - the service-approved destination requirement is removed.
- `gateway-stack-catalog-egress`:
  - the scope adds request accounts;
  - the static checks apply at BYOK decode;
  - the settings are renamed;
  - the documentation and evidence requirements cover BYOK.
- `gateway-provider-configuration`: BYOK construction may use the account's policy-admitted `baseURL`, and OpenAI Responses models use the egress client for it.
- `gateway-resource-policy`: request accounts with a `baseURL` use the egress client under the same bounds.

## Impact

- **Code:**
  - `ai-gateway/pkg/internal/outbound`: the endpoint check, the egress model wrapper and the two-client choice;
  - `ai-gateway/pkg/internal/stackcatalog`: calls the shared check;
  - `ai-gateway/pkg/internal/byok`: `DecodeRequest` takes the policy, and `New` takes both clients;
  - `ai-gateway/pkg/internal/service`: `BYOKOptions` and `StackCatalogOptions` share the clients;
  - `ai-gateway/pkg/internal/config`: the setting names;
  - `ai-gateway/pkg/internal/gateway`: wiring;
  - e2e compose and `grafana.ini`: the renamed keys.
- **APIs:** request accounts accept more `baseURL` values. No new field, error document, category or status. Two settings are renamed before release.
- **Tests:** `byok`, `service` and gateway harness tests in cloud mode; `test/providerwire-v4` scenarios through both clients. The Playwright environment runs `self-hosted`, where callers get configured access, so it has no BYOK path (see design).
- **Not parity-sensitive:** upstream `@ai-sdk/gateway` 4.0.132 types `providerOptions.gateway.byok` as an opaque `Record<string, Array<Record<string, unknown>>>` that "the Gateway service validates". It forwards account fields without interpreting them, so the accepted `baseURL` values are Gateway policy.
