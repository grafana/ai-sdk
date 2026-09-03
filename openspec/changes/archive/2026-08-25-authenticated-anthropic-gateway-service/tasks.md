## 1. ProviderWire Host Error Authority

- [x] 1.1 Add `ai-gateway/providerwire/v4` tests for exact fixed authentication, permission, and internal documents plus invalid-category internal fallback without runtime schema or byte-limit behavior.
- [x] 1.2 Implement the non-fallible narrow typed host-safe error writer by reusing package-owned fixed documents without accepting configuration, messages, causes, or dynamic encoding.
- [x] 1.3 Add compile-time/API tests proving the host writer exposes no private DTO, arbitrary status, code, type, retryability, or byte-limit control.

## 2. Isolated AGPL Service and Concrete Process Settings

- [x] 2.1 Add `ai-gateway/cmd/grafana-ai-gateway` inside the existing isolated AGPL Go module with a minimal command and internal packages for config, outbound HTTP, auth, discovery, service construction, and process lifecycle.
- [x] 2.2 Keep `ai-gateway` excluded from `go.work` while repository build, test, short-test, vet, lint, tidy, boundary, and module-resolution tasks execute it independently with `GOWORK=off` and no root dependency-graph change.
- [x] 2.3 Pin Kingpin, authlib, Prometheus, strict YAML, and published rewritten root/Anthropic prerequisite revisions in `ai-gateway/go.mod` without local replacements; assert the root module graph remains unchanged.
- [x] 2.4 Add test seams for arguments, environment lookup, listener creation, shutdown signals, clock/timers, logs, metrics, and executable process control.
- [x] 2.5 Add table-driven tests for every documented flag, explicit environment name, default, deployment/auth enum, positive value, safe `limit+1` constraint, and numeric TCP listener syntax.
- [x] 2.6 Implement typed Kingpin parsing with no package-global parser state and exact environment bindings from the design table, excluding lower-stack limits that no longer exist.
- [x] 2.7 Add tests for checked write-budget addition, duration overflow, the default `30s + 5s + 120s + 5s <= 165s` relationship, and cross-field idle/header timeout constraints.
- [x] 2.8 Implement staged process validation so invalid listener syntax and scalar JWKS policy fail before YAML, secret, client, or listener work.

## 3. Strict YAML, Secrets, and Public IDs

- [x] 3.1 Add failing tests for bounded YAML reads, exact-limit/over-limit files, known-field enforcement, duplicate keys, trailing documents, provider/model shapes, empty model sets, and reference failures.
- [x] 3.2 Implement one-document strict YAML decoding and staged named-provider/model validation.
- [x] 3.3 Add table-driven tests for the exact 1-128 byte public ID grammar, including canonical IDs and aliases with whitespace, controls, commas, non-ASCII, invalid punctuation, and boundary lengths.
- [x] 3.4 Implement header-safe public ID validation before catalog construction.
- [x] 3.5 Add tests that `apiKeyEnv` is required, resolved exactly once, and reported without values when unset/empty, and that literal credential fields are rejected as unknown.
- [x] 3.6 Implement injected environment-secret resolution without retaining environment maps or printing secret values.

## 4. Hardened Outbound HTTP

- [x] 4.1 Add production/development URL-policy tests for scheme and absolute host plus rejection of userinfo, opaque URLs, query, forced query, fragment, and empty host for both JWKS and Anthropic.
- [x] 4.2 Implement the shared credential-free hierarchical endpoint validator and independent cloned JWKS/Anthropic transports with named dial/TLS/expect-continue/idle constants plus exact `MaxIdleConns: 32`, `MaxIdleConnsPerHost: 8`, and `MaxConnsPerHost: 32`; inspect both transports in tests.
- [x] 4.3 Add redirect tests for same-origin and cross-origin targets that capture whether `X-Access-Token`, `Authorization`, or `x-api-key` leaks.
- [x] 4.4 Configure both clients to reject redirects before credential forwarding.
- [x] 4.5 Add exact-limit/over-limit tests for plain and gzip-expanded JWKS, Anthropic unary success/error, one oversized SSE line, and a multiline SSE event whose aggregate exceeds the configured decompressed bound.
- [x] 4.6 Implement the post-decompression bounded response body that errors on the first byte above the limit and closes the underlying body.
- [x] 4.7 Add hostile server tests for dial/response-header hangs, stalled bodies, request-context cancellation, and Anthropic cumulative stream-byte exhaustion.
- [x] 4.8 Wire configured response-header/JWKS request timeouts and prove all hostile outbound tests terminate within their bounds.

## 5. Bounded JWKS Snapshot and Authentication

- [x] 5.1 Add deterministic-clock tests for empty/duplicate key IDs, malformed keys, non-200/content failures, byte/key-count bounds, exact maximum keys, refresh success, failed-refresh preservation, maximum age, and no eager startup fetch.
- [x] 5.2 Add concurrent unique-`kid` flood tests proving at most one outbound refresh per minimum interval, no unknown-key retention, and bounded snapshot/flight state.
- [x] 5.3 Add barrier-based rotated-key and canceled-starter tests proving all uncanceled callers join or observe one successful service-owned refresh while every caller can stop waiting on its own context.
- [x] 5.4 Implement the service-owned `authn.KeyRetriever` with one immutable bounded snapshot and mutex-owned flight; reserve cadence, launch one service-context-plus-timeout refresh, and make starter and joiners all wait on `done` versus their request contexts.
- [x] 5.5 Add header-boundary tests for case variants, multiple field lines/values, comma coalescing, empty values, optional exact `Bearer ` stripping, and Authorization-only requests.
- [x] 5.6 Implement exact-one `X-Access-Token` and at-most-one `X-Grafana-Id` normalization before authlib using a private token provider.
- [x] 5.7 Add auth construction tests for production bounded-JWKS sharing, default/custom audiences, unsafe development verification/warning, production unsafe rejection, and contradictory JWKS settings.
- [x] 5.8 Implement access/ID verifier construction over the shared bounded retriever and the explicit unsafe development path.
- [x] 5.9 Add identity tests requiring exactly one non-empty `authn.ServiceIdentityKey`, rejecting absent/empty/multiple values, and proving acting-user subject/identifier can never become caller service.
- [x] 5.10 Add context/privacy tests proving acting-user extraction is independent and normalized context retains no `AuthInfo`, extras map, raw/parsed token, ID-token extra, email, group, or permission data.
- [x] 5.11 Implement protected-route middleware that authenticates before decoding/listing/resolution, populates request telemetry state, and writes fixed host-safe V4 401 errors without logging raw authlib errors.

## 6. Anthropic Provider, Models, and Closed Discovery

- [x] 6.1 Add direct-provider poisoned-environment tests for conflicting `ANTHROPIC_BASE_URL`, API key/auth token, explicit and fallback profiles, federation/organization/identity-token sources, and custom headers across unary and streaming calls.
- [x] 6.2 Correct `providers/anthropic.New` to pass `option.WithoutEnvironmentDefaults()` and `option.WithAPIKey(apiKey)` directly to `anthropic.NewClient`, preserving the separate Vertex path and explicit request options.
- [x] 6.3 Add construction tests for resolved named provider configuration, hardened-client injection, optional allowed base URLs, missing references, one-time model construction, and stable canonical/alias model identity.
- [x] 6.4 Resolve named Anthropic provider configuration directly and build one immutable static catalog with reviewed HTTP-client/base-URL request options.
- [x] 6.5 Add a test-only closed draft 2020-12 discovery schema and tests for required fields, fixed `v4`/`grafana` values, additional-property rejection, invalid UTF-8, and private-field exclusion.
- [x] 6.6 Add discovery encoder tests below/at/above its byte limit, large descriptions repeated across many aliases, listing failures, no partial 200 commitment, and host-safe internal fallback.
- [x] 6.7 Implement private discovery DTOs plus row/field/string incremental `limit+1` encoding with immediate early stop and precommit response handling; keep schema validation test-only.
- [x] 6.8 Add discovery projection tests for authentication ordering, canonical/alias complete rows, alias `modelId`, stable sorting, names/descriptions, and absence of provider names/types, backend IDs, credentials, env names, base URLs, and topology.
- [x] 6.9 Implement authenticated `/config` projection from `catalog.ModelLister`.
- [x] 6.10 Add pinned `@ai-sdk/gateway@4.0.52` coverage that consumes discovery and invokes every returned canonical/alias `modelId` successfully through header construction and resolution.

## 7. Exact Routing, Flush-Safe Telemetry, and Lifecycle

- [x] 7.1 Add route tests for exactly five supported method/path pairs, explicit `HEAD /config` rejection, 404 behavior, `Allow: GET`/`Allow: POST` on every recognized-path 405, prefix stripping, no legacy route, and zero protected work for unsupported requests.
- [x] 7.2 Implement the standard-library dispatcher with unauthenticated `/live`, `/ready`, and authoritative-plan `/metrics` plus shared auth around config/language-model routes and explicit Allow headers.
- [x] 7.3 Add telemetry normalization tests for exactly six route labels (`live`, `ready`, `metrics`, `config`, `language_model`, `unmatched`), three method labels (`GET`, `POST`, `other`), and five status-class labels.
- [x] 7.4 Add tests proving arbitrary paths/methods, callers, namespaces, model IDs, provider data, URLs, and errors never become metric labels or unbounded log fields.
- [x] 7.5 Implement one outer request-scoped telemetry state owner that records every outcome exactly once and permits auth to populate normalized caller fields.
- [x] 7.6 Add response-writer tests for `Unwrap() http.ResponseWriter`, status capture, wrapped `ResponseController` flushing, and exactly-once auth-failure telemetry.
- [x] 7.7 Implement the unwrap-capable response writer and verify ProviderWire sees the underlying flush capability.
- [x] 7.8 Add a real-listener test that receives the first ProviderWire frame while an authenticated stream remains open and silent, then confirms telemetry is emitted once at termination.
- [x] 7.9 Add Prometheus registration/scrape tests for Go/process, readiness, in-flight, count, and duration collectors, including duplicate registration and forbidden value searches.
- [x] 7.10 Implement one service-owned registry and `/metrics` handler with only normalized route/method/status labels.
- [x] 7.11 Add readiness tests proving listener-bound local construction, no JWKS/Anthropic probes, and transition to unready before request cancellation.
- [x] 7.12 Add `http.Server.MaxHeaderBytes` boundary tests below the configured value, within the documented 4096-byte parser slop where syntactically possible, and above the effective read bound.
- [x] 7.13 Implement atomic liveness/readiness state and configured `http.Server` read/write/idle/header bounds while documenting that the header setting is Go's parser value, not an exact wire ceiling.
- [x] 7.14 Add shutdown tests that separately prove graceful completion with a cancellation-aware stream and force-close after deadline with a cancellation-ignoring handler.
- [x] 7.15 Implement cancel-first shutdown with `Server.Shutdown` derived from `context.Background()` or `context.WithoutCancel`, followed by force-close only on failure/deadline.
- [x] 7.16 Add focused and real-command tests for exactly-once fixed startup, ready, shutdown-start, and shutdown-complete records plus configuration/provider secret exclusion.
- [x] 7.17 Implement fixed privacy-safe process lifecycle events at the four required transitions.

## 8. Real-Command Registered-Client Evidence

- [x] 8.1 Build a separate fake-Anthropic HTTP fixture process/server that asserts SDK path, API-key auth, backend model, ordered text/scalars, response bounds, and request cancellation without using provider conformance directories.
- [x] 8.2 Add integration harness code that runs `go build` for `ai-gateway/cmd/grafana-ai-gateway`, writes temporary YAML, supplies only documented flags/environment, races public readiness against early process exit, and cleans temporary state on either outcome.
- [x] 8.3 Add a real-command authenticated discovery/unary scenario that invokes every canonical/alias row, reaches fake Anthropic exactly once per call, and accepts the lower stack's minimal unary content/usage/finish output without requiring replaced response metadata.
- [x] 8.4 Add a real-command normal streaming scenario where fake Anthropic emits initial/text/finish events and the pinned client observes finish plus clean EOF while the command remains running.
- [x] 8.5 Add a separate real-command client-abort scenario where fake Anthropic emits one valid initial event, the client receives the initial ProviderWire frame while open, then aborts and the fake observes request cancellation while the command remains ready.
- [x] 8.6 Add a separate real-command process-shutdown scenario using a fresh established silent stream, then SIGTERM, readiness failure, Anthropic request cancellation, graceful independent-deadline shutdown, and bounded process exit.
- [x] 8.7 Add process assertions for malformed-scalar startup failure, Authorization-only rejection, duplicate-token rejection, discovery overflow, outbound redirect/oversize privacy, and absence of credentials/backend values in responses, logs, discovery, and metrics.
- [x] 8.8 Prove the integration imports no in-process service/router/catalog/authenticator construction and executes only the built command path.

## 9. Parity and Repository Validation

- [x] 9.1 Update `test/conformance/PARITY.md` for the direct-provider environment-default security deviation, authenticated service/frontend interoperability, strict discovery authority, authoritative operational `/metrics`, preserved Anthropic base-URL normalization gap, and fake-transport evidence boundaries.
- [x] 9.2 Run `gofmt`, ProviderWire tests, isolated `ai-gateway` service tests, root/Anthropic tests, `mise run test-providerwire-v4`, and the AGPL-owned real-command integration suite.
- [x] 9.3 Run `mise run validate-parity-baseline`, `mise run parity-check`, standalone `ai-gateway` build/vet/lint, boundary and module-resolution checks, and `mise run check`; confirm no provider input fixture or unrelated generated file changed.
- [x] 9.4 Review the final adapted diff against every work-package-5 acceptance and security boundary after the accepted work-package-4 rebase.
