# Design

## Context

- Not parity-sensitive; see the proposal for the upstream check (`@ai-sdk/gateway` 4.0.132 types `byok` as opaque, service-validated records).
- **BYOK today** (`pkg/internal/byok/request.go`, `validateAccounts`):
  - a non-empty `baseURL` must equal `nativemodel.AnthropicBaseURL` (`https://api.anthropic.com`) or `OpenAIBaseURL` (`https://api.openai.com/v1`) exactly, or appear in `approvedBaseURLs[provider]`;
  - `service.selectBYOK` passes `nil`, so production accepts only the exact native URL;
  - every supplied account is checked, including accounts of providers the request doesn't use;
  - a mismatch wraps `byok.ErrInvalidRequest`, then becomes `execution.ErrInvalidBYOK`, `CategoryBYOKCredentials` and the fixed 400 document "providerOptions.gateway.byok requires supported provider arrays with valid accounts";
  - `byok.New` builds every candidate with the shared model client, whose `ProxyFromEnvironment` applies.
- **Stack catalogs today:**
  - `stackcatalog.checkEndpoints` runs `outbound.ValidateEndpoint`, requires `https`, runs `EgressPolicy.CheckHost` and normalizes the URL. It names the field path on failure.
  - `SelectStackCatalog` picks `EgressClient` when `BaseURL != ""` and wraps that candidate in `service.egressCandidate`, which returns `ErrEgressRefused`, `ErrEgressConnection` or `ErrEgressTimeout` bare.
  - A request-time static failure is `ErrCatalogConfiguration` (424). A connect-time refusal is `ErrEgressRefused` (424), classified per attempt under the aggregate chain rule.
- **Imports:**
  - `byok` imports `config`, `nativemodel`, `nativeoptions`, `routing` and `execution`, but not `service` or `stackcatalog`;
  - `service` imports `byok`;
  - `stackcatalog` imports `v1alpha1`;
  - `outbound` imports only `execution` and the standard library. It can take the `provider` interface (root module) without a cycle.
- The Playwright environment runs `access_mode = self-hosted` (`provisioning/grafana.ini`). BYOK requires `cloud` mode, where only listed internal services get configured access.

## Goals / Non-Goals

**Goals:**

- One endpoint rule for both customer sources, implemented once: the same static check, client choice and error rewrite.
- BYOK keeps its decode-time contract: invalid accounts fail before construction with the BYOK credentials document.

**Non-Goals:**

- Redacting BYOK request keys in failure summaries; #771 settles that.
- A BYOK-only opt-in setting or a separate BYOK policy.
- Endpoints for the operator catalog, which keeps its rules.

## Decisions

### Shared code: everything lives in `outbound`

- `outbound.CheckEndpoint(raw, native string, policy EgressPolicy) (string, error)`: the current `checkEndpoints` body for one URL. `native` is the provider type's native default (`nativemodel.DefaultBaseURL(type)`, empty for `openai-compatible`). It returns:
  - `""` when the normalized URL equals the native default (scheme and host compared case-insensitively, a trailing `/` ignored). The provider then counts as having no `baseURL`, before the scheme and policy checks, so a deny entry can't refuse the native endpoint;
  - otherwise the normalized URL, or one of these sentinels:
  - `ErrEndpointForm` (`ValidateEndpoint` failure);
  - `ErrEndpointScheme` (not `https`);
  - `ErrEndpointHost` (the unexported `errEndpointHost`, exported);
  - `execution.ErrEgressRefused`.

  It makes no DNS lookup.
  - `stackcatalog` maps the sentinels to its field-path messages, which stay byte-identical.
  - `byok` maps every sentinel to `ErrInvalidRequest` with a fixed reason that never contains the URL.
- `outbound.EgressModel(model provider.LanguageModel) provider.LanguageModel`: today's `service.egressCandidate` and `bareEgressError`, moved unchanged.
- `outbound.ModelClients{Shared, Egress *http.Client}` and `For(baseURL string) (client *http.Client, custom bool)`. This is the single rule: a non-empty `baseURL` that `CheckEndpoint` returned is custom and gets the egress client. Callers wrap the model with `EgressModel` when `custom`. The native comparison lives in `CheckEndpoint`, next to `For`, because it must also skip the policy.
- `service.StackCatalogOptions` and `service.BYOKOptions` each hold a `Clients outbound.ModelClients` and an `Egress outbound.EgressPolicy`. `StackCatalogOptions.Client` and `EgressClient` go away.
- Alternatives considered:
  - `byok` importing `stackcatalog`: that pulls the kind types into BYOK, and the field-path messages don't fit request controls.
  - A new package: three small functions don't need one, and `outbound` already owns the policy and both clients.
  - The wrapper in `execution`: it sits next to the sentinels there, but it belongs with the client whose errors it rewrites.

### BYOK decode

- `byok.DecodeRequest(selector string, controls routing.Controls, policy outbound.EgressPolicy)` replaces the `approvedBaseURLs` parameter.
- For every supplied account with a non-empty `baseURL` (unused providers included, as today), `validateAccounts` calls `outbound.CheckEndpoint` and stores the normalized URL in the account's `nativemodel.Config`.
- A failure wraps `ErrInvalidRequest` as `"%w: baseURL must be an https URL the egress policy permits"`. This is internal text only: the caller still gets the fixed BYOK credentials document, and the URL never appears.
- The exact-native-URL shortcut goes away. The native URLs pass the check like any public `https` endpoint.

### BYOK construction

- `byok.New(request, placements, clients outbound.ModelClients)`.
- `newCandidate` calls `clients.For(account.BaseURL)` and builds the native model with that client, keeping today's owned-client copy that rejects redirects.
- When the endpoint is custom, it wraps the translated model in `outbound.EgressModel`, outside `execution.Translate` and inside fallback, at the same position as the stack candidate.
- Fallback across accounts, the attempt bound and native option checks are unchanged. A refused account is an ordinary attempt failure under the aggregate chain rule.

### Error mapping (BYOK compared with stack catalogs)

| Failure | BYOK | Stack catalog |
|---|---|---|
| URL form, scheme, host form, refused literal, deny hostname (no DNS) | decode: `ErrInvalidBYOK` → BYOK credentials, 400 `invalid_request_error`/`invalid_request`, fixed message, no provider request | admission: Kubernetes 400 naming the field path; per request: catalog configuration, 424, fixed message |
| Resolved address refused, or the host doesn't resolve | attempt: `ErrEgressRefused` → egress refused, 424 `failed_dependency`, not retryable | the same |
| Connection, TLS or read failure after the policy permitted the address | `ErrEgressConnection` → upstream 502; `ErrEgressTimeout` → timeout 504 | the same |

Who can fix it decides the class. A BYOK caller fixes its own request (400), and a stack writer fixes the catalog (400 at write, 424 at call). Failures at connect time are identical for both.

### Setting names

`stack_catalog_egress_allow` and `stack_catalog_egress_deny` become `custom_endpoint_allow` and `custom_endpoint_deny`:
- Go: `config.EgressAllowSetting` and `EgressDenySetting` change value;
- the `grafana.ini` keys and the e2e compose variables change with them.

They haven't been released, and once released the stack-only names would outlive the rule they describe. The capability keeps its name `gateway-stack-catalog-egress`; its Purpose is updated to cover request accounts.

### The native default counts as unset

A `baseURL` equal, after normalization, to its type's native default counts as unset for both sources. It uses the shared client and its proxy, and the egress lists don't apply. That keeps BYOK callers who send the native URL explicitly working in proxy-only deployments, as they do today. It's one comparison in `CheckEndpoint`, tested for both sources. For stack catalogs it is a small change: an explicit native URL used the egress client before and now uses the shared client, like a provider without `baseURL`.

## Test plan

| Behavior | Evidence |
|---|---|
| A public `https` `baseURL` works | `byok` decode table, public and native URLs accepted and normalized. `byok.New` with recording clients: a custom URL uses `Egress` and is wrapped; no URL uses `Shared`. Gateway harness in cloud mode with allow `127.0.0.1`: a BYOK account at a loopback TLS fake answers unary and stream with its key. |
| Static refusals at decode | `byok` decode table: `http`, userinfo, query, `127.0.0.1`, `169.254.169.254`, `[::ffff:10.0.0.1]`, `2130706433`, a deny hostname, an unused provider's refused URL. Each wraps `ErrInvalidRequest`, with no URL in the message. The `service` selector maps it to `ErrInvalidBYOK`. Harness: the fixed 400 document, zero provider requests. |
| Connect-time refusal | Harness: `https://localhost:<port>` without an allow entry gets the 424 egress document with zero requests at the listener. A refused first account followed by a working second account succeeds. |
| Bare errors | `outbound.TestEgressModel` (synthetic) and `TestSelectStackCatalog_EgressErrorsAreBare`, which stays in `service` because it drives the three real provider SDKs. A BYOK harness overview contains no URL or address. |
| Shared rule | `outbound.CheckEndpoint` and `ModelClients.For` tables. The existing `stackcatalog` messages are unchanged (`TestParse_RefusesEndpoints`). |
| Settings rename | Settings tests for the new keys; the old keys are ignored, as removed settings are. |
| Cross-language | `test/providerwire-v4` `startCloudGateway`: both clients get 400 for a refused literal and 424 for a refused loopback name, with zero connections. |
| Native default counts as unset | `CheckEndpoint` table (exact, case, trailing slash, other path, other port). Stack `Parse` and BYOK decode each store `""` for it, and both service paths pick the shared client with recording clients, also under a deny entry for the native host. |
| Not covered by e2e | Playwright runs `self-hosted`, where users and service accounts get configured access and `byok` is refused. A BYOK e2e needs a second Grafana in `cloud` mode: another compose service, settings and a Playwright project. The plugin-protocol suite already runs the built plugin in cloud mode with both clients, which is the same path below Grafana's route handler, and Grafana treats `byok` as request body. |

## Risks / Trade-offs

- **[Who can trigger egress widens]** In cloud mode, any user or service account can now make the Gateway connect to any permitted public host and port. Before, only stack-catalog writers (Admins) could. Mitigations: the same policy and operator deny prefixes, the attempt bound of 8, and Grafana's request limits. The README states it.
- **[Proxy-only deployments]** Accounts and stack providers with a custom `baseURL` can't connect there; the native default, set or not, keeps the proxy. Documented.
- **[Hostile endpoints echo request keys into overviews]** The caller owns those keys; redaction of request keys is #771's decision.
- **[#771 refactor in flight]** It may touch `service` selection and options; this change merges `nrbrd/758-stack-catalog-routes` when it lands and rebuilds the options on top.

## Migration Plan

No data migration. The settings rename takes effect before release; operators who tried the branch's `stack_catalog_egress_*` keys must rename them. Rolling back restores native-only BYOK; accounts that used custom endpoints then fail at decode with the BYOK credentials document.

## Decisions recorded at review

1. Settings renamed to `custom_endpoint_allow` and `custom_endpoint_deny`.
2. A `baseURL` equal to its type's native default, after normalization, counts as unset for both sources and keeps the shared client and proxy.
3. The decode refusal keeps the fixed BYOK credentials 400.
4. No BYOK e2e: Playwright runs `self-hosted`, which has no BYOK path. The cloud-mode plugin-protocol suite with both clients is the evidence, and the gap is recorded here.
5. No opt-in setting; the README security note states the widened set of callers.
