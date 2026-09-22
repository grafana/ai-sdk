## Context

The application currently requires `X-Access-Token` and optionally verifies `X-Grafana-Id` through authlib. It ignores accompanying `Authorization`. The application needs a separate mode for identity supplied by a trusted authenticating reverse proxy.

`internal/process` currently constructs JWKS and Anthropic clients together and owns one HTTP server. `internal/service` combines protected discovery and model routes with unauthenticated operational routes. The provider credential comes from application configuration, independently of caller authentication.

This change defines application work only. Deployment owners control edge configuration, enforced ingress, and activation. Image publication remains separate.

### Registered baseline recheck

Source review used the exact registered Vercel commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`. No baseline mismatch was found.

| Evidence | Finding |
| --- | --- |
| `test/conformance/upstream.yaml` | Registered versions include `@ai-sdk/gateway@4.0.52`, `ai@7.0.65`, `@ai-sdk/provider@4.0.7`, and `@ai-sdk/provider-utils@5.0.27`. |
| Upstream `packages/gateway/src/gateway-provider.ts` and its tests | Explicit `apiKey` becomes a Bearer credential without a token exchange. Configured headers participate in request construction. |
| Upstream `packages/gateway/src/gateway-language-model.ts` | The client removes `abortSignal` from the body but retains call-level `headers` and provider options. |
| Upstream `packages/gateway/src/gateway-provider-options.ts` | `byok` is a separate provider-options representation. Client serialization does not establish service support. |
| Upstream `packages/ai/src/generate-text/` | `generateText` adds model-call headers; `streamText` forwards supplied headers. Both default tool choice to `auto`. |
| `ai-gateway/providerwire/v4/request.go` | The mapper rejects nonempty body headers, tool choice, and nonempty provider options. |
| `ai-gateway/test/providerwire-v4/gateway-command.test.ts` | Command evidence uses internal authentication and a local Cloud edge shim, with low-level calls and explicit `maxOutputTokens`. |
| `test/conformance/PARITY.md` | This work belongs to ProviderWire host composition, not provider conformance or Vercel private-service parity. |

[Registered Gateway source](https://github.com/vercel/ai/tree/d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e/packages/gateway/src) is the client reference. Cloud authentication has no upstream service equivalent. It is a Grafana host extension. Cloud command composition is covered by local shim tests. Real edge authorization and ingress enforcement still require deployment evidence. High-level text calls and default unary token limits remain separate compatibility gaps. No package upgrade, request rewriting, or provider fixture changes are needed.

## Goals / Non-Goals

### Goals

- Accept trusted reverse-proxy identity in a startup-selected mode.
- Preserve internal JWT verification and the default combined listener.
- Keep identity fields distinct and credentials private.
- Remove Cloud-mode dependence on JWKS.
- Separate API access from operational access and coordinate both servers.
- Prove application behavior with the registered low-level client and deterministic local fixtures.

### Non-Goals

- Proxy credential verification or access-policy evaluation in the application.
- Cluster-policy inspection or deployment trust verification in application code.
- Request-scoped BYOK, credential storage, provider fallback, or billing policy.
- Native OpenAI/Anthropic adapters or alternate Cloud-header implementation or naming.
- High-level `generateText`/`streamText` support or default unary token limits.
- Changes to reusable SDK modules, provider modules, authlib, or baseline versions.
- Deployment resources, image publication, production rollout, quotas, aggregate capacity controls, or public-preview readiness.
- Drain-before-cancel shutdown or authenticated internal transport implementation.

## Decisions

### Keep proxy authentication separate from provider credentials

The reverse proxy must authenticate and authorize requests, replace client-supplied identity headers, and remove client credentials before forwarding. The application validates the identity headers. Client credential formats and proxy implementation are outside this application's contract.

Provider credentials remain in application configuration. `ResolveProviderSecrets` and `BuildCatalog` remain unchanged. Requests cannot select provider keys, base URLs, backend models, or mutate shared clients.

Request-scoped BYOK and native OpenAI/Anthropic adapters are unsupported. Their future credential contracts are outside this change.

### Introduce one request-authentication boundary

`internal/auth` owns this interface:

```go
type RequestAuthenticator interface {
    Authenticate(context.Context, http.Header) (Caller, error)
}
```

Headers keep authentication independent of request-body decoding. The `access-token` adapter reuses `normalizeHeaders`, `exactlyOneHeader`, authlib verification, and `callerFromAuthInfo`. It preserves audience, namespace, signature, service identity, and optional acting-user checks.

The middleware receives an authentication failure writer instead of owning protocol-specific formatting. Current ProviderWire routes inject `HostErrorWriter` and retain the fixed authentication document. Future native adapters can inject native errors without changing identity validation.

Middleware completes authentication before protected body reads, discovery, model resolution, or provider calls. Authentication failure never tries another mode. Internal mode continues ignoring accompanying `Authorization`, even when that header looks like a Cloud credential.

A composite authenticator that tries JWT and then trusted headers would let failed credentials cross trust boundaries. Startup selection avoids that fallback.

### Validate assertions and retain distinct identity

Cloud mode requires exactly one value for each header:

| Header | Meaning and validation |
| --- | --- |
| `X-Scope-OrgID` | Target stack ID: positive decimal digits fitting `int64`. |
| `X-Cloud-Org-ID` | Policy organization ID: positive decimal digits fitting `int64`. |
| `X-Access-Policy-ID` | Nonempty opaque policy identifier; UUID syntax is not required. |

Reuse `exactlyOneHeader`. Reject missing, empty, duplicate, case-colliding, comma-coalesced, control-character, and invalid-whitespace values. Do not trim malformed assertions into valid ones. Numeric assertions reject signs, nondigits, zero, and overflow. Policy identifiers do not require UUID syntax.

Extend `Caller` with a typed authentication source and separate private fields for stack ID, policy organization ID, and policy ID. Derive the namespace with the existing `types.CloudNamespaceFormatter` from `github.com/grafana/authlib/types` pinned at `0d62418c2815`.

A Cloud caller has no manufactured service identity or acting user. The policy ID is not `Caller.Service`. The policy organization is not the target stack.

The Cloud ProviderWire entry point rejects surviving `Authorization`, `X-Access-Token`, or `X-Grafana-Id` before body reads. Presence indicates an incorrect edge handoff. Apply that restriction only to this mode and adapter; native adapters may later carry provider credentials in native headers.

UUID-only parsing would reject valid edge identifiers. Reusing internal service fields would erase distinct identity meanings. Both alternatives are excluded.

### Make construction and validation mode-aware

Add `--auth.mode=access-token|cloud-gateway`, bound to `GRAFANA_AI_GATEWAY_AUTH_MODE`, defaulting to `access-token`. Reject unknown modes at startup.

Cloud mode rejects `auth.unsafe` and a nonempty JWKS URL. It skips JWKS endpoint validation, client construction, verifier construction, and retrieval. JWKS-specific limits apply only to the JWT path.

Split `outbound.NewClients` into independent JWKS and Anthropic constructors. Preserve transport bounds, redirect rejection, endpoint validation, timeouts, and response-size protection for each constructed client.

The API write-timeout minimum remains an overflow-checked sum. Internal mode retains read timeout, JWKS timeout, model duration, and response grace. Cloud mode excludes the JWKS term. Provider and general server limits remain enforced.

Building an unused JWKS client would still couple Cloud startup to irrelevant validation. Ignoring contradictory JWT configuration would hide operator mistakes. Separate construction and explicit rejection avoid both problems.

### Separate dispatch and coordinate server ownership

Add `--server.operational-listen-address`, bound to `GRAFANA_AI_GATEWAY_SERVER_OPERATIONAL_LISTEN_ADDRESS`, with an empty default.

| Mode | Operational address | Behavior |
| --- | --- | --- |
| `access-token` | Unset | Existing combined listener. |
| `access-token` | Set | Separate API and operational listeners. |
| `cloud-gateway` | Unset | Startup fails. |
| `cloud-gateway` | Set | Separate listeners required. |

Every configured listener must use loopback when unsafe development authentication is enabled. Preserve its development-mode requirement.

Exact API dispatch serves `GET /api/v1/aisdk/config` and `POST /api/v1/aisdk/language-model`. Exact operational dispatch serves `GET /live`, `GET /ready`, and `GET /metrics`. Neither dispatcher invokes handlers belonging to the other listener. Preserve method checks and encoded-path rejection. Retain a combined handler for internal compatibility.

The process constructs dependencies and binds all listeners before readiness. If a second bind fails, it closes the first listener. Either unexpected serving failure stops both servers and returns the failure.

Shutdown withdraws readiness before canceling requests. Both servers share one shutdown deadline. Force-close on deadline expiry and reap both serving goroutines. Do not sequentially grant each server a full timeout.

Keep one telemetry registry and existing HTTP metrics. Authentication observations use fixed source and outcome values, never credential or customer-ID labels. Preserve `responseWriter.Unwrap` so server-sent event flushing works through middleware.

Keeping metrics on the Cloud API port would force scrapers into the trusted ingress set. Separate ports let deployment owners grant operational access without granting API access.

### Separate three kinds of command evidence

Extend the existing real-command suite rather than replacing the service with handler mocks. The local edge shim uses fixed dummy credentials and read/write outcomes, not CAP verification or production scope evaluation.

The shim receives the stack-qualified Bearer credential, checks a fixed test outcome, strips Cloud/internal credentials, replaces all three identity headers, and forwards the unchanged path.

| Scenario family | Injection point | Required evidence |
| --- | --- | --- |
| Edge denial | Dummy credential or configured scope outcome at the shim | Invalid credentials, read-only inference, and write-only discovery stop with zero application and provider calls. No application error-format claim. |
| Client spoofing | Client identity headers before shim replacement | Valid dummy credentials proceed using the shim's values, not the client's. |
| Malformed trusted assertions | Controlled fixture after shim replacement | Missing, duplicate, case-colliding, coalesced, and invalid values fail application authentication before body reads or provider work. |

Use low-level `getAvailableModels`, `doGenerate`, and `doStream`; both model calls supply explicit `maxOutputTokens`. Do not rewrite high-level requests to remove rejected body fields.

Fake Anthropic must receive only its configured provider credential, never CAP credentials, internal JWTs, identity assertions, or incoming provider keys. Capture application authentication responses, logs, and metrics to assert fixed errors and absence of private values. Test cancellation, incremental flushing, startup failures, listener separation, internal JWT regression, and dual-server shutdown.

Fake edge and provider responses are integration fixtures, not recorded provider conformance inputs. Deployed proxy authorization and ingress isolation require separate verification.

## Risks / Trade-offs

- Header syntax cannot authenticate the sender. Deployment owners must verify that only trusted proxy peers can reach the API listener.
- Two listeners add lifecycle failure paths. Mitigation: bind before readiness, close partial startup, share one deadline, and test goroutine cleanup.
- Cancel-first shutdown can interrupt active streams during rollout. Mitigation: document cancellation; longer pod termination grace does not promise stream draining.
- Successful low-level tests could imply unsupported normal SDK use. Mitigation: label high-level text calls and default unary token limits as unresolved onboarding blockers.
- Identity observations could expose customer data. Mitigation: fixed-value telemetry and negative assertions across responses, logs, metrics, and outbound headers.

## Migration Plan

Application implementation follows the five phases in `tasks.md`. Default internal authentication and single-listener operation remain compatible. The main specification is synced after implementation and verification.

Deployment configuration and rollout procedures are outside this repository. Before enabling Cloud mode, verify trusted proxy-only API access and separate operational access. Local tests do not establish deployment safety. Rollback must preserve authentication and API isolation.
