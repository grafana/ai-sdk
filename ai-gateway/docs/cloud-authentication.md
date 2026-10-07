# Listener and deployment authentication contract

The command always builds private JWT, trusted-Cloud and operational listeners.
For client setup and BYOK semantics, use the central
[authentication guide](../../docs/guides/gateway-authentication.md).

## Listener separation

| Listener | Default | Policy and routes |
| --- | --- | --- |
| `server.private-listen-address` | `:8080` | Verified JWT; configured discovery and inference |
| `server.cloud-listen-address` | `:8081` | Trusted edge assertions; request-only BYOK inference, unsupported discovery |
| `server.operational-listen-address` | `:8082` | Unauthenticated `/live`, `/ready`, `/metrics` |

The corresponding environment variables use the
`GRAFANA_AI_GATEWAY_SERVER_*_LISTEN_ADDRESS` prefix. There is no process-wide
`auth.mode`, combined listener or legacy listener alias. API listeners serve
`GET /api/v1/aisdk/config` and `POST /api/v1/aisdk/language-model`; they do not
serve operational routes, and the operational listener does not serve API routes.

Regional JWKS trust is required for the private API. Audience is `ai-sdk`;
concrete stack and wildcard namespaces are supported without a service allowlist.
The private API rejects mixed credentials and Cloud identity assertions before
reading the body. Development-only unsafe verification requires loopback addresses
for every listener; it is not a deployment authentication mechanism.

## Cloud trust boundary

The existing authenticating edge remains responsible for CAP credentials,
`ai-gateway:read`/`ai-gateway:write` scope, realm/stack and IP validation. No
backend-enterprise JWT ingress or token-verification change is required.

The edge must replace client-supplied `X-Scope-OrgID` with exactly one authenticated
positive decimal stack ID fitting `int64`. The application rejects missing,
duplicate, comma-separated and malformed values, including whitespace.
`X-Cloud-Org-ID` and `X-Access-Policy-ID` are not authoritative and are ignored on
this listener. Before forwarding, strip `Authorization`, `X-Access-Token` and
`X-Grafana-Id`; any surviving header causes application authentication failure.

Identity headers do not authenticate their sender. **Only the authenticating edge
may reach the Cloud application port**, including restrictions against otherwise
trusted internal workloads. Private API clients must not be able to forge Cloud
assertions by dialing that port. A private ClusterIP or namespace-wide ingress
allowance alone is not sufficient. Do not expose the private JWT listener through
public ingress. Restrict the operational listener independently to monitoring and
probe clients.

BYOK is authorized by the authenticated account policy, not by recognizing a header
or finding credentials in a request. It does not consult configured catalogs,
accounts, aliases, endpoints or fallback routes. Native Anthropic/OpenAI keys are
request-scoped, native endpoints are fixed, redirects are disabled and no account
fallback crosses this boundary.

## Lifecycle and activation proof

Readiness waits for all three listeners. Bind/startup failure closes listeners
already created. Shutdown cancels active calls from both populations before all
servers stop under one shared shutdown deadline; provider/export cleanup retains
its bounded budgets. Set the deployment termination grace period accordingly.

The separately authorized deployment-tools delivery must provide rendered Services,
private-only routing, regional JWKS and configured-secret references, and NetworkPolicy
for all three ports. Before activation, retain deployed positive and negative
connectivity evidence: authorized private JWT access, edge-only Cloud access,
internal-client denial on the Cloud port, no public JWT route, and operational
access only from approved observers/probes. Repository manifests and localhost
fixtures are not deployed network-isolation proof.

Local tests exercise the real command with both Go and pinned Vercel clients, a
dummy edge, and synthetic native responses over a local TLS proxy. They establish
application behavior, not production CAP authentication or deployed isolation.

[Up: Grafana AI Gateway](../README.md)
