# Cloud authentication

At startup, Grafana AI Gateway selects exactly one authentication mode: JSON Web
Token (JWT) verification or identity headers supplied by an authenticating reverse
proxy. A request that fails the selected mode is rejected; the gateway does not try
the other mode. Provider credentials come from server configuration in both modes. For Go and
server-side Vercel client credentials and examples, see [Authenticate to Grafana
AI Gateway](../../docs/guides/gateway-authentication.md).

## Authentication modes

Set `--auth.mode` or `GRAFANA_AI_GATEWAY_AUTH_MODE`:

| Mode | Caller authentication |
| --- | --- |
| `access-token` (default) | Verify `X-Access-Token` as a JWT. If `X-Grafana-Id` is present, also verify the acting user. |
| `cloud-gateway` | Validate `X-Scope-OrgID` supplied by a trusted reverse proxy. |

In `access-token` mode, `Authorization` does not replace `X-Access-Token`.
Invalid JWTs do not fall back to trusted headers.

In `cloud-gateway` mode, the application neither authenticates client credentials
nor evaluates access policies. The reverse proxy must authenticate and authorize
every request before forwarding it. This mode rejects `--auth.unsafe` and any
nonempty JSON Web Key Set (JWKS) URL, and it does not fetch signing keys.

## Trusted proxy requirements

Use deployment network controls to ensure that only the authenticating proxy can
reach the API listener. Identity headers do not authenticate their sender. Verify
those controls before enabling `cloud-gateway` mode, and never expose the API
listener directly to untrusted clients.

The proxy must replace client-supplied `X-Scope-OrgID` with the authenticated
stack ID.
The application requires exactly one value for this header:

| Header | Meaning | Accepted value |
| --- | --- | --- |
| `X-Scope-OrgID` | Target stack ID | Positive decimal digits fitting `int64`. |

The application rejects missing, duplicate, comma-separated, or malformed
`X-Scope-OrgID` values. It does not trim whitespace to make an invalid value acceptable.

`X-Cloud-Org-ID` and `X-Access-Policy-ID` are not required. The application ignores
them, even when present.

Before forwarding a request to the Cloud ProviderWire API listener, the proxy must
remove `Authorization`, `X-Access-Token`, and `X-Grafana-Id`. In `cloud-gateway`
mode, the application rejects any request that still contains one of these headers.
Authentication finishes before the application reads the request body or calls a
provider.

## Separate operational access

The `cloud-gateway` mode requires a separate operational listener, configured with
`--server.operational-listen-address` or
`GRAFANA_AI_GATEWAY_SERVER_OPERATIONAL_LISTEN_ADDRESS`. Apply access controls to
that listener separately from the trusted API listener.

| Listener | Routes |
| --- | --- |
| API | `GET /api/v1/aisdk/config`, `POST /api/v1/aisdk/language-model` |
| Operational | `GET /live`, `GET /ready`, `GET /metrics` |

Neither listener serves the other listener's routes. Operational routes are
unauthenticated, so restrict access to monitoring and probe clients.
In `access-token` mode, omitting the operational address preserves the combined listener.

During shutdown, the gateway cancels active requests before waiting for both
servers to stop. Increasing the deployment's termination grace period does not
keep active streams from being canceled at that point.

## Client compatibility

Configure clients with the endpoint and credentials required by the authenticating
proxy, as described in the [shared authentication guide](../../docs/guides/gateway-authentication.md).
Clients must not bypass the proxy and send those credentials directly to
the application's `cloud-gateway` listener. Keep provider credentials in server-side
configuration; never place them in browser code or logs.

Supported client calls include `getAvailableModels`, `doGenerate`, and
`doStream`. Calls to `doGenerate` and `doStream` must set
`maxOutputTokens` explicitly.

`generateText` and `streamText` both set `toolChoice` to `auto`. The unary mapper
accepts that choice, while streaming and fallback routes reject it. `generateText`
also adds unsupported body headers, and `streamText` only forwards supplied headers.
Setting `maxOutputTokens` does not make either high-level call compatible.

Bring-your-own-key (BYOK) requests and public OpenAI/Anthropic API adapters
are not supported.

Local integration tests use a dummy proxy and fake provider responses.
The test proxy replaces `X-Scope-OrgID` with its stack assertion and strips other client identity headers and credentials.
These tests cover application behavior, not a deployed proxy's authentication or network isolation.

[Up: Grafana AI Gateway](../README.md)
