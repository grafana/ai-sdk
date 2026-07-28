## Context

`providers/grafana/provider.go` exposes a single constructor `NewWithCloudAuth`, which requires `CAPToken` + `TokenExchangeURL` + `Namespace` + `BaseURL`. Internally it builds an `authn.TokenExchangeClient` that exchanges the CAP for short-lived access tokens on every model call (TTL-cached by authlib). The model attaches the resulting JWT as `X-Access-Token` on the provider-wire HTTP request.

A `WithTokenExchanger(authn.TokenExchanger)` functional option exists, documented as "intended for tests", and is the only seam available to bypass the local CAP→AT exchange. The k6 case wants exactly that — but with a static, pre-minted access token — and today has to construct a placeholder cloud config plus implement the full `authn.TokenExchanger` interface to wrap a literal string. That ergonomics gap is the issue.

Two relevant authlib facts shape this design:

1. `authn.NewStaticTokenExchanger(token string)` already exists and returns an `authn.TokenExchanger` that yields the same token regardless of request. We can reuse it verbatim — no provider-local "tiny static exchanger" needed.
2. `Namespace` and `Audience` are only ever consumed by the exchanger (`Exchange(ctx, {Namespace, Audiences})`). The provider-wire HTTP request to the hosted endpoint contains no namespace/audience headers — those values live as claims inside the access-token JWT, baked in by whoever minted it. For a pre-minted access token, `Namespace` and `Audience` configuration on the provider would be inert decoration that invites confusion ("can these override the JWT claims? what if they disagree?").

The hosted endpoint (`grafana-assistant-app/api/internal/aisdkprovider/auth.go`) verifies `X-Access-Token` and optionally `X-Grafana-Id` via authlib's JWKS-backed verifiers. The two-token OBO pattern works today. The single-token OBO pattern (user identity in the access token's `act` claim) is gated incorrectly on raw-token presence at the server side and silently drops `Caller.User` — tracked as a separate fix in [grafana-assistant-app#6764](https://github.com/grafana/grafana-assistant-app/issues/6764). This change targets the two-token pattern only.

## Goals / Non-Goals

**Goals:**

- Let consumers construct a working Grafana provider given a pre-minted access token, without holding a CAP or implementing `authn.TokenExchanger`.
- Keep the existing `NewWithCloudAuth` constructor and the `WithUserIDToken` context helper unchanged.
- Share the existing `Provider` struct and HTTP/SSE codepath. The new constructor only changes how the access-token string is sourced.
- Match upstream authlib's surface symmetry: `NewTokenExchangeClient` is "I'll mint tokens for you"; `NewStaticTokenExchanger` is "I already have one". The provider should expose the same symmetry one level up.

**Non-Goals:**

- A function-based `AccessTokenSource` constructor (Option B from issue #198). Not needed for the driving use case; can be added non-breakingly later if a real consumer asks.
- Server-side support for the single-token act-claim OBO pattern. Tracked separately.
- Changing the `X-Access-Token` / `X-Grafana-Id` wire contract or the `WithUserIDToken` context helper.
- Refresh hooks or caller-pluggable token sources. The caller is responsible for re-minting before the access token expires; the provider just forwards whatever it has.

## Decisions

### Decision 1: Reuse `authn.NewStaticTokenExchanger` rather than introduce a provider-local static exchanger

`authlib/authn/token_exchange.go` already ships `StaticTokenExchanger`. Wiring it as the `Provider.tokenExchanger` for static-token providers means the model's existing `accessToken(ctx)` path (`Exchange(ctx, {Namespace, Audiences})`) keeps working unchanged — the static exchanger just ignores the request.

**Alternatives considered:**

- Introduce a private `staticTokenExchanger` type in the provider package. Rejected: duplicates upstream functionality for no benefit; deviates from authlib's surface, which is the canonical reference per `AGENTS.md`.
- Refactor the model to skip the exchanger entirely when a static token is available. Rejected: forks the model's HTTP request path on auth mode; more code; harder to keep behaviorally identical between the two constructors.

### Decision 2: `AccessTokenConfig` omits `Namespace` and `Audience`

A pre-minted access token has its namespace and audience baked into its JWT claims. The hosted endpoint never sees them on the wire — they're verified against `authn.VerifierConfig.AllowedAudiences` by the server, but that's fixed on the server, not configurable by the client.

Including them on `AccessTokenConfig` would be dead config at best and a footgun at worst (suggesting they could override JWT claims). They're omitted. This deviates from issue #198's proposal but matches the actual wire reality.

**Trade-off:** Slightly asymmetric with `CloudAuthConfig`. Documented in `doc.go` / `README.md` with rationale; the asymmetry reflects the underlying auth-mode difference rather than hiding it.

### Decision 3: Remove `WithTokenExchanger` from the public surface

It was documented as "intended for tests" and was the workaround driving issue #198. Once `NewWithAccessToken` exists, no production caller has a legitimate reason to inject a custom exchanger — any custom auth flow either fits one of the two constructors or warrants its own follow-up constructor.

Per `AGENTS.md`, we don't aim for backward compatibility unless asked. Removing it now cleans the public surface before any external caller comes to rely on it.

For internal tests we add a package-private constructor seam (e.g. `newProviderForTest(t *testing.T, exchanger authn.TokenExchanger, ...) *Provider`) that builds the `Provider` struct directly. This keeps test fakes out of the public API.

**Alternatives considered:**

- Keep `WithTokenExchanger` as a documented escape hatch. Rejected: production use cases now have explicit constructors, and the option's existence would invite the same awkward placeholder-CAP pattern the issue is removing.
- Mark `WithTokenExchanger` as `Deprecated:` and remove later. Rejected: per project preference, "implement the change without considering the need of being backward compatible, unless asked".

### Decision 4: Ship Option A only (static token); defer Option B (function-based source)

YAGNI. The k6 case is squarely Option A — testcoordinator mints, hands off, k6 forwards. Option B's caller-pluggable refresh hook has no known consumer today. Adding it later as `NewWithAccessTokenSource(AccessTokenSourceConfig{...})` would be a non-breaking addition.

### Decision 5: `Provider` struct stays shared, with `namespace` and `audience` zero-valued for static-token providers

The model calls `tokenExchanger.Exchange(ctx, authn.TokenExchangeRequest{Namespace: p.namespace, Audiences: []string{p.audience}})`. For a `StaticTokenExchanger`, both arguments are ignored — the static token is returned regardless. So zero-valued `namespace`/`audience` are harmless.

**Alternatives considered:**

- Split `Provider` into `cloudProvider` and `staticProvider` (or hide behind an interface). Rejected: invasive refactor for an internal field that has no externally observable effect. The shared struct keeps the diff small and matches issue #198's "sharing the same underlying `Provider` struct".

### Decision 6: Constructor validates `AccessToken` is non-empty and `BaseURL` is a valid HTTP(S) URL

Same validation philosophy as `NewWithCloudAuth`: fail at construction time, not on the first call. `AccessToken` non-empty check mirrors the `CAPToken` non-empty check. `BaseURL` reuses the existing `normalizeBaseURL` helper. No format validation on the access token itself — keep it opaque; if it's malformed or expired, the hosted endpoint returns 401 with a non-retryable `APICallError`, which is the right surface.

## Risks / Trade-offs

**[Risk] Callers don't refresh the access token before expiration** → The provider has no refresh hook, by design. Expired tokens produce 401s from the hosted endpoint with non-retryable `APICallError`. Mitigation: document clearly in `README.md` that the caller owns refresh; recommend short-lived process scope (CI, test runners, jobs); flag that retries won't help on auth failure.

**[Risk] Caller passes a CAP token to `NewWithAccessToken` thinking it's "an access token in the broad sense"** → The hosted endpoint validates `typ=jwt+at` via authlib's `AccessTokenVerifier`, so a CAP token (different type) will be rejected with a clear auth error. Mitigation: doc comment on `AccessTokenConfig.AccessToken` explicitly names "short-lived access token JWT (`typ=jwt+at`) minted via auth-api `/v1/sign-access-token`".

**[Risk] Removing `WithTokenExchanger` breaks an external caller we don't know about** → Per `AGENTS.md` we don't optimize for backward compatibility unless asked. The option was documented as test-only. Mitigation: clear changelog/PR description; provide a migration note pointing at `NewWithAccessToken`.

**[Trade-off] `AccessTokenConfig` asymmetric with `CloudAuthConfig`** → Documented with rationale. The asymmetry reflects the underlying truth: cloud-auth needs `Namespace`/`Audience` for the exchange request; access-token mode doesn't, because those claims are inside the JWT. Hiding that under uniform-but-inert fields would mislead callers.

**[Trade-off] User-identity attribution still requires two tokens** → Until grafana-assistant-app#6764 lands, callers wanting per-user attribution must forward both `X-Access-Token` and `X-Grafana-Id` via `WithUserIDToken`. The k6 team has accepted this; documented in `README.md` under "User Identity Forwarding".
