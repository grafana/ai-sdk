## 1. Public API surface

- [x] 1.1 Add `AccessTokenConfig` struct in `providers/grafana/provider.go` with fields `AccessToken string`, `BaseURL string`, `HTTPClient *http.Client`, and a doc comment naming the expected token type (`typ=jwt+at` minted via auth-api `/v1/sign-access-token`).
- [x] 1.2 Add `NewWithAccessToken(cfg AccessTokenConfig, opts ...Option) (*Provider, error)` that validates `AccessToken` is non-empty, validates `BaseURL` via the existing `normalizeBaseURL` helper, applies functional options, wires `authn.NewStaticTokenExchanger(cfg.AccessToken)` as the `tokenExchanger`, leaves `namespace` and `audience` zero-valued, and returns the shared `*Provider` struct.
- [x] 1.3 Remove the public `WithTokenExchanger` functional option and any associated exported symbols.
- [x] 1.4 Update `providerOptions` to drop the `tokenExchanger` field (no longer reachable from public options).

## 2. Internal test seam

- [x] 2.1 Add a package-private constructor in `providers/grafana/` (for example `newTestProvider(baseURL string, exchanger authn.TokenExchanger, opts ...testOption) *Provider`) that builds the `Provider` struct directly for white-box tests.
- [x] 2.2 Migrate `provider_test.go` test sites that previously used `WithTokenExchanger` to the new test seam; verify all existing tests still compile and pass.

## 3. Documentation

- [x] 3.1 Update `providers/grafana/doc.go` to mention both auth modes (cloud-auth and static access token) and link to the relevant constructor docs.
- [x] 3.2 Update `providers/grafana/README.md`:
  - rename the existing "Cloud Auth" section to clarify it's one of two modes
  - add a "Pre-minted Access Token" section showing `NewWithAccessToken` usage
  - document that the caller owns access-token refresh
  - clarify that `WithUserIDToken` still works in both modes for the two-token OBO pattern
- [x] 3.3 Add doc comments on `AccessTokenConfig` fields covering required vs optional, expected token type/lifetime, and the lack of `Namespace`/`Audience` knobs (with rationale).

## 4. Tests

- [x] 4.1 Add `TestNewWithAccessToken_ValidationAndDefaults` covering empty `AccessToken`, empty `BaseURL`, invalid `BaseURL` (non-http(s) scheme, query, fragment), and `BaseURL` trailing-slash normalization.
- [x] 4.2 Add `TestNewWithAccessToken_NoExchangeHTTP` asserting that across multiple `DoStream` / `DoGenerate` calls the provider issues no HTTP traffic to any URL other than `BaseURL + wire.PathLanguageModel`. Use an httptest server for the provider-wire endpoint and an unexpected-request httptest server that fails the test if hit.
- [x] 4.3 Add `TestNewWithAccessToken_HeaderPropagation` asserting that `X-Access-Token` equals the configured static string, `X-Grafana-Id` is absent without `WithUserIDToken`, and `X-Grafana-Id` is present when `WithUserIDToken` is used on the call context.
- [x] 4.4 Add `TestNewWithAccessToken_RegistryAndModelMetadata` mirroring the existing cloud-auth registry/middleware tests to confirm behavioral parity.
- [x] 4.5 Add `TestNewWithAccessToken_StreamAndGenerateParity` running a representative stream and generate case through a fake hosted endpoint and asserting the same `StreamPart` / `GenerateResult` decoding behavior as the cloud-auth path.
- [x] 4.6 Verify `make test` passes (root and providers modules) and `make vet` is clean.

## 5. Cleanup verification

- [x] 5.1 Grep the providers/grafana package for any remaining references to `WithTokenExchanger` or `tokenExchanger` as a public concept and confirm none exist outside the package-private seam.
- [x] 5.2 Run `make check` (fmt + vet + test) end-to-end and confirm a clean exit.
- [x] 5.3 Review the public API diff (e.g. `go doc -all ./providers/grafana`) and confirm the new surface is exactly: `Provider`, `Option`, `WithHTTPClient`, `CloudAuthConfig`, `AccessTokenConfig`, `NewWithCloudAuth`, `NewWithAccessToken`, `WithUserIDToken`. Specifically confirm `WithTokenExchanger` is gone.
