## ADDED Requirements

### Requirement: Pre-minted access token constructor

The provider SHALL expose a constructor `NewWithAccessToken(cfg AccessTokenConfig, opts ...Option) (*Provider, error)` for consumers who already hold a short-lived access token JWT minted out-of-band (for example by a separate process calling auth-api `/v1/sign-access-token`). The constructor MUST share the existing `Provider` struct with `NewWithCloudAuth` and MUST produce a provider whose `LanguageModel` returns models behaviorally identical to those of `NewWithCloudAuth`, differing only in how the per-call access token is sourced.

#### Scenario: Access-token constructor available alongside cloud-auth

- **WHEN** a consumer reads the public API surface of `providers/grafana`
- **THEN** both `NewWithCloudAuth` and `NewWithAccessToken` are exported and documented as the two supported ways to construct a provider

#### Scenario: Returned provider satisfies the same contract

- **WHEN** a consumer constructs a provider via `NewWithAccessToken` and resolves a `LanguageModel`
- **THEN** the model exposes the same `SpecificationVersion`, `Provider`, `ModelID`, `SupportedURLs`, `DoStream`, and `DoGenerate` behavior as a model from `NewWithCloudAuth`

### Requirement: Access token configuration

`AccessTokenConfig` SHALL include exactly the fields required for the static-token mode:

- `AccessToken` (string, required): pre-minted short-lived access token JWT to forward as `X-Access-Token` on every model call.
- `BaseURL` (string, required): base URL of the Grafana hosted ai-sdk provider-wire endpoint.
- `HTTPClient` (`*http.Client`, optional): override for the underlying HTTP client used for provider-wire requests.

`AccessTokenConfig` MUST NOT include `Namespace` or `Audience` fields. Those values are claims inside the pre-minted JWT and are never sent on the wire to the hosted endpoint by the provider in this mode.

#### Scenario: Required fields are validated at construction

- **WHEN** either `AccessToken` or `BaseURL` is empty
- **THEN** `NewWithAccessToken` returns an error rather than producing a misconfigured provider

#### Scenario: BaseURL validation matches the cloud-auth constructor

- **WHEN** `BaseURL` is not a valid `http://` or `https://` URL (for example, contains a query string, fragment, or unsupported scheme)
- **THEN** `NewWithAccessToken` returns an error consistent with `NewWithCloudAuth`'s URL validation

#### Scenario: No namespace or audience knobs

- **WHEN** a consumer reads the `AccessTokenConfig` struct
- **THEN** the struct has no `Namespace`, `Audience`, or related fields

### Requirement: Access token mode does not perform CAP exchange

A provider constructed via `NewWithAccessToken` SHALL NOT call any token-exchange URL. It MUST source every per-call access token from the configured static string via `authn.NewStaticTokenExchanger`. The provider MUST NOT mint, sign, parse, or otherwise inspect the access token; it is treated as an opaque bearer.

#### Scenario: No exchange HTTP traffic

- **WHEN** a consumer makes any number of `DoStream` or `DoGenerate` calls against a provider constructed via `NewWithAccessToken`
- **THEN** the provider issues HTTP requests only to the configured `BaseURL` provider-wire endpoint and never to a token-exchange URL

#### Scenario: Access token forwarded as bearer

- **WHEN** the provider makes a provider-wire HTTP call
- **THEN** the request carries `X-Access-Token: <AccessTokenConfig.AccessToken>` exactly as supplied at construction

## MODIFIED Requirements

### Requirement: Constructor naming reflects cloud-only auth

The provider SHALL expose `NewWithCloudAuth(cfg CloudAuthConfig, opts ...Option) (*Provider, error)` for the cloud-auth mode where the calling process holds a Cloud Access Policy token and exchanges it locally for short-lived access tokens. The constructor MUST accept a typed configuration value carrying cloud auth parameters and MAY accept functional options for future non-breaking knobs. The constructor name MUST make the cloud-auth mode explicit so it remains distinguishable from alternative auth-mode constructors (for example `NewWithAccessToken`) on the same `Provider`.

#### Scenario: Cloud-auth constructor name

- **WHEN** a consumer reads the public API surface
- **THEN** the cloud-auth constructor is named `NewWithCloudAuth`, explicitly identifying its auth mode

### Requirement: Token exchange uses authlib

The provider SHALL acquire the per-call access token via an `authn.TokenExchanger` from `github.com/grafana/authlib/authn`. For providers constructed via `NewWithCloudAuth`, the exchanger MUST be an `authn.TokenExchangeClient` built from the configured `CAPToken` and `TokenExchangeURL`. For providers constructed via `NewWithAccessToken`, the exchanger MUST be `authn.NewStaticTokenExchanger(cfg.AccessToken)`. The provider MUST NOT mint or sign tokens itself and MUST NOT implement its own token cache. The provider MUST NOT expose any public functional option for injecting custom `authn.TokenExchanger` implementations.

#### Scenario: Cloud-auth provider exchanges via authlib

- **WHEN** a provider constructed via `NewWithCloudAuth` makes a provider-wire HTTP call
- **THEN** the access token attached as `X-Access-Token` was obtained via `authn.TokenExchangeClient.Exchange` using the configured CAP and token-exchange URL

#### Scenario: Access-token provider uses static exchanger

- **WHEN** a provider constructed via `NewWithAccessToken` makes a provider-wire HTTP call
- **THEN** the access token attached as `X-Access-Token` was obtained via `authn.StaticTokenExchanger` and equals `AccessTokenConfig.AccessToken`

#### Scenario: Token caching is delegated to authlib

- **WHEN** multiple cloud-auth calls happen in close succession
- **THEN** the provider relies on `authlib`'s built-in token caching and does not implement its own

#### Scenario: No public token-exchanger injection

- **WHEN** a consumer reads the public functional-options surface of `providers/grafana`
- **THEN** there is no exported `Option` that injects a custom `authn.TokenExchanger`
