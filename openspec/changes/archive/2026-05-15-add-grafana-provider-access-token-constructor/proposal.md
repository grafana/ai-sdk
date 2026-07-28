## Why

`providers/grafana` today exposes only `NewWithCloudAuth(cfg, opts...)`, which requires every calling process to hold a Cloud Access Policy (CAP) token and a `TokenExchangeURL` so authlib can exchange the CAP for short-lived access tokens locally. That's a sensible default for services that own the policy, but it forces a security coupling consumers explicitly want to avoid: the calling process must hold the long-lived CAP.

The k6 + ai-sdk integration is the driving case. Their k8s-side testcoordinator holds the CAP and mints short-lived `aud=ai-sdk` access tokens via auth-api's `/v1/sign-access-token`. Their EC2-side load-generator binaries should only ever see the minted access token. Today, to wire this up they must pass placeholder strings into `NewWithCloudAuth` and override `WithTokenExchanger` with a hand-rolled `authn.TokenExchanger` that returns a literal string. That's awkward enough to discourage the safer pattern.

We also have `authn.NewStaticTokenExchanger` already in authlib, which is exactly the shape we want internally. The provider just doesn't expose a constructor that wires it.

## What Changes

- Add `NewWithAccessToken(cfg AccessTokenConfig, opts ...Option) (*Provider, error)` to `providers/grafana` for callers who hold a pre-minted access token directly. The new constructor wires `authlib`'s `NewStaticTokenExchanger` internally and reuses the existing `Provider` struct.
- `AccessTokenConfig` carries only the fields that matter for this auth mode: `AccessToken` (required), `BaseURL` (required), `HTTPClient` (optional). `Namespace` and `Audience` are intentionally absent — they are claims inside the pre-minted JWT and never appear on the wire to the hosted endpoint.
- **BREAKING**: Remove the `WithTokenExchanger` functional option. It was documented as "intended for tests" and was the workaround for this use case; with `NewWithAccessToken` available the workaround stops being necessary. Internal tests switch to constructing providers directly with a fake exchanger via an unexported test seam.
- Keep `WithUserIDToken(ctx, idToken)` unchanged. Both constructors continue to support the standard two-token OBO pattern (`X-Access-Token` + `X-Grafana-Id`).
- Update `README.md` and `doc.go` to document both constructors and when to use each.

## Capabilities

### New Capabilities

None. This change extends an existing capability.

### Modified Capabilities

- `grafana-provider`: add a static-access-token constructor; remove the `WithTokenExchanger` test-only option from the public surface.

## Impact

- **Public API surface**: new `NewWithAccessToken` and `AccessTokenConfig`. Removal of `WithTokenExchanger` is breaking for any external caller using it; per `AGENTS.md` we don't aim for backward compatibility unless asked, and the only documented use was tests.
- **Internal**: the `Provider` struct keeps its current fields. For static-token providers, `namespace` and `audience` are zero/empty and are passed harmlessly into `StaticTokenExchanger.Exchange`, which ignores them.
- **Dependencies**: no new dependencies; `authn.NewStaticTokenExchanger` already lives in the authlib version we use.
- **Tests**: replace `WithTokenExchanger`-based test wiring with a package-private constructor seam. Add a `TestNewWithAccessToken_*` group covering validation, header propagation, and absence of any token-exchange HTTP traffic.
- **Server side (orthogonal)**: the single-token OBO improvement (act-claim) lives in `grafana-assistant-app` ([issue #6764](https://github.com/grafana/grafana-assistant-app/issues/6764)). This change targets only the two-token pattern, which is fully supported by the hosted endpoint today.
- **No wire/server coordination needed**: `NewWithAccessToken` sends the exact same HTTP request shape as `NewWithCloudAuth`; only the source of the access-token string changes.
