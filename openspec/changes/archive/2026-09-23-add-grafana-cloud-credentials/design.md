## Context

The deployed path is client → cortex-gw → AI Gateway in `cloud-gateway` mode. Cortex-gw accepts `Authorization: Bearer <stack-id>:<CAP-token>`, authenticates the policy, enforces route scopes and instance access, and forwards a replaced `X-Scope-OrgID` after removing credentials. The application trusts only that protected hop. Direct CAP support requires no server authentication changes.

The Go provider currently always acquires a JWT and sends `X-Access-Token`. `NewWithCloudAuth` exchanges a CAP through authlib; `NewWithAccessToken` forwards a caller-managed JWT. These remain useful for access-token endpoints but do not authenticate the current public Cloud route.

### Reference and evidence

- Registered upstream: `@ai-sdk/gateway@4.0.87`, `ai@7.0.107`, commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d`. Its `packages/gateway/src/gateway-provider.ts` and tests confirm that explicit `apiKey` becomes `Authorization: Bearer ${apiKey}` without exchange. Discovery and model calls use the same header factory.
- Local contract: `providers/grafana/provider.go`, `openspec/specs/grafana-gateway-client/spec.md`, and `openspec/specs/ai-gateway-cloud-authentication/spec.md`.
- Edge reference: `backend-enterprise` at `caceb1ad8c`, `pkg/authentication/authorization.go`, `pkg/authentication/grafanacloud/authenticator.go`, and `pkg/gateway/routes.go`.
- Deployment reference: `deployment_tools` at `d9e5efd57a7a`, `ksonnet/lib/ai-gateway/{main,runtime,network-policy}.libsonnet`. The CAP creation helper defaults to system scope when org/realms are omitted; caller provisioning must not inherit that scope accidentally.
- Coverage classification: provider implementation/request projection and Gateway host composition. `test/conformance/PARITY.md` records these as partial/mixed evidence, explicitly not deployed authentication proof.

Existing prose contains older baseline numbers and unqualified claims that Authorization cannot authenticate the Gateway. Rewrite the touched authentication guidance against the manifest; do not upgrade pins or expand unrelated protocol support.

## Goals / Non-Goals

**Goals:**

- Authenticate Go discovery and generation against the existing public Cloud edge using a CAP directly.
- Distinguish direct CAP, exchanged JWT and pre-minted JWT flows in the API and user documentation.
- Establish equivalent direct-CAP header behavior for Go and the pinned Vercel client without credential leakage or auth fallback.
- Preserve existing JWT behavior under unambiguous names.

**Non-Goals:**

- No server authentication, payload, listener or deployment topology changes.
- No new JWT verifier or signing service, automatic token-type detection, CAP refresh/cache, or browser authentication.
- No solution for k6 delegated session credentials, per-user Cloud identity, or hosted rollout inside this repository change.

## Decisions

### 1. Three explicit typed constructors

Add `NewWithCloudCredentials(CloudCredentialsConfig, ...Option)` with `StackID int64`, `CAPToken string`, and the existing common `BaseURL`, `HTTPClient`, `Headers` and `Limits` fields. Require a positive stack ID and a nonempty header-safe opaque CAP token. Do not assume a particular CAP prefix or parse token contents. A numeric stack ID avoids confusing Cloud organization IDs and `stacks-<id>` namespaces with the decimal transport identifier.

Rename `NewWithCloudAuth` / `CloudAuthConfig` to `NewWithTokenExchange` / `TokenExchangeConfig`, without aliases. Preserve namespace, default audience, authlib caching, cancellation and error sanitization. Leave `NewWithAccessToken` / `AccessTokenConfig` unchanged.

All constructors retain existing HTTP(S) URL validation, immutable client/header copies, limits, no ambient credential discovery, and redirect refusal. Production guidance requires HTTPS; local HTTP test endpoints remain supported.

Alternative rejected: overloading the existing constructor or detecting JWT/CAP formats. Those obscure endpoint compatibility and make accidental authentication fallback possible.

### 2. Direct CAP is a separate request-authentication path

CAP calls construct exactly one `Authorization: Bearer <decimal-stack-id>:<CAP-token>` header for discovery, unary and streaming requests. No authlib exchanger is constructed or called for this path; requests do not pass through the exchange gate. Authentication material is applied only to outer HTTP headers, not the ProviderWire body or result metadata. Keep the implementation private and small; no exported authentication framework is needed.

JWT constructors continue to acquire/send `X-Access-Token` and support context-carried acting users. Their existing handling of optional outer Authorization headers is not changed by this feature.

Alternative rejected: attaching an Authorization header to the current exchange flow. That still requires signing permissions, internal Auth API access and a second, unused credential.

### 3. Fail explicitly on Cloud credential/identity conflicts

For the CAP constructor, configured and call-level headers must not contain `Authorization`, `X-Access-Token`, `X-Grafana-Id`, `X-Scope-OrgID`, `X-Cloud-Org-ID`, or `X-Access-Policy-ID`, case-insensitively. Reject configured conflicts at construction and call-level conflicts before serialization/network I/O. This protects the selected credential and prevents sensitive call headers from being copied into the ProviderWire `headers` body member. Non-reserved call headers retain existing projection semantics and may still be rejected by the server's supported subset.

A nonempty context value from `WithUserIDToken` is an error for CAP requests. An empty value remains absent. Cloud mode currently conveys stack identity, not a verified acting user; silently stripping a user supplied through the API would misrepresent delegation.

Alternative rejected: silently overwriting every conflicting header. Although upstream allows header overrides, the Go client deliberately owns its auth boundary. The CAP rejection policy is an intentional, documented extension of that stricter boundary, not an upstream parity claim. Typed constructor configuration is a parity-preserving Go adaptation for ordinary valid calls.

### 4. Reuse the existing command/edge test architecture

Extend the Go capture executable with explicit Cloud-credentials and token-exchange inputs, rejecting ambiguous selection. Compare Go with Vercel configured using `apiKey: "<stack-id>:<dummy-cap>"`. Assert auth headers at the edge and separately assert credential removal and stack replacement on the backend hop. Do not put auth in body-carried call headers to make tests pass.

Cover discovery, low-level unary and streaming requests, cancellation, no exchange traffic, configured dummy scope denials, invalid credentials, redirect refusal and identity-header conflicts. Keep direct raw-request spoofing tests at the edge; the typed client should reject such headers. Preserve JWT/exchange regressions under the new names.

Use deterministic transport tests and existing ProviderWire request captures. Do not invent provider recordings or modify recorded provider inputs. Request-body and provider-response snapshots should remain unchanged; review any changed expectations rather than regenerating broadly.

Local tests exercise our boundary contract with a dummy edge, not real CAP validation. Live scope, realm, expiry/revocation and network-isolation checks remain a separately authorized deployment acceptance step.

### 5. Rewrite the user authentication story for Go and Vercel

Create `docs/guides/gateway-authentication.md` as the shared task-oriented guide, linked from `docs/README.md`, the Go provider guide, container guide and existing Gateway Cloud-authentication guide. Replace duplicate/misleading client-auth sections with short explanations and links. Keep server trust/listener requirements in their existing guide and reference that guide rather than duplicating it; do not broaden this into a server-doc relocation.

The shared guide owns:

- A credential/endpoint matrix: CAP through public cortex-gw; JWT against an explicitly access-token-enabled endpoint; browser UI through the application's backend. Distinguish application login, gateway access and server-owned model-provider credentials.
- Working Go CAP and server-side Vercel `createGateway` examples, including the `/api/v1/aisdk` prefix and catalog discovery. Use tested low-level generation/streaming options rather than implying all high-level APIs work.
- Go token-exchange and pre-minted JWT examples with endpoint restrictions, default audience/namespace explanations, refresh ownership and the breaking rename migration.
- An explicit statement that Vercel's `apiKey` does not perform Grafana token exchange. A JWT supplied there is not equivalent to Go's `X-Access-Token` flow; do not invent an untested TypeScript exchange recipe.
- Read/write scopes, stack versus organization identity, least-privilege realms, secure storage, expiry/rotation, HTTPS and redaction. No system CAP in browser code or customer-controlled workers.
- Troubleshooting credential/header mismatch versus scope/realm denial, configuration errors versus remote failures, and authenticated-but-unsupported model calls. Do not promise exact HTTP statuses beyond the tested edge contract.
- A clear supported-now versus out-of-scope boundary for acting users and k6 delegated sessions.

Keep exhaustive API reference in godoc. Examples use placeholders, no internal deployment URLs or real tokens, and must be compiled/typechecked and exercised by deterministic tests against the pinned packages.

## Risks / Trade-offs

- Longer-lived CAP crosses the edge on each request → use HTTPS, least privilege, operator-managed expiry/rotation, no redirects and no credential-bearing call metadata.
- Broad system policy accidentally reused → document explicit realm provisioning and require an external rollout review; the client cannot infer policy privileges.
- Breaking constructor/type rename affects consumers → migrate all repository-owned uses atomically and identify external consumers before release. Do not keep misleading aliases.
- Client-selected stack ID can be wrong or malicious → cortex-gw remains responsible for authorizing that stack; syntax validation is not authorization.
- Local success mistaken for hosted acceptance → preserve the coverage gap and separate deployment verification from merge evidence.
- Auth success mistaken for full ProviderWire compatibility → examples stay within the tested model-call subset and link existing limitations.

## Migration Plan

1. Land failing CAP wire tests, the typed constructor and rename, repository call-site updates, documentation and deterministic evidence together.
2. Publish the client module with migration instructions; verify module resolution outside the workspace. Consumers retaining JWT endpoints adopt the renamed constructor/type only.
3. In separate `deployment_tools`/consumer changes, provision CAP policies with reviewed realms/scopes and adopt the direct-CAP constructor or Vercel configuration. Remove exchange settings only from migrated consumers. No server rollout is required for this authentication feature.
4. Deployment owners verify both clients through real cortex-gw, including rejected credentials/scopes/realms, credential stripping and backend isolation. Do not read secrets or run these checks without authorization.
5. Rollback a consumer by restoring its previous known-working client/configuration or disabling the new integration and revoking the new policy. The previously broken public JWT route is not a fallback. No server auth-mode rollback is needed.

## External owner handoff and live acceptance

The `deployment_tools` owners should provision a new policy with reviewed stack realm and `ai-gateway:read`/`ai-gateway:write` scopes, store the CAP in an appropriate secret, and update each consumer's URL and constructor or Vercel `apiKey`. The `app-examples` example application uses the old exchange constructor and needs a coordinated upgrade to the direct CAP path if it uses the public URL. The `grafana-assistant-app` repository has old-constructor test call sites; coordinate its dependency upgrade and rename without changing deployed credentials implicitly. `k6-agentic-browser-testing` uses `NewWithAccessToken` for a worker's pre-minted JWT, so its authentication migration remains separate. Exact release timing and consumer ownership must be confirmed with those teams before publishing a breaking module version.

Deployment owners must verify, without publishing credential values: successful read discovery and write unary/streaming from both clients through real cortex-gw; rejected invalid, expired and revoked CAPs; wrong stack/realm and insufficient read/write scopes; replacement of forged `X-Scope-OrgID`, stripping of CAP and JWT headers before forwarding; and direct backend listener isolation. Record versions and safe response outcomes. Local shim tests are not proof of these properties. Rollback is a consumer/credential change, not an auth-mode change in the server.

## Open Questions

- External consumer owners and release timing must be confirmed before publishing the breaking rename; their code changes are not included here.
- k6's worker trust boundary and short-lived delegated credential design remain a separate decision with IAM. Neither blocks direct CAP support nor is solved by it.
