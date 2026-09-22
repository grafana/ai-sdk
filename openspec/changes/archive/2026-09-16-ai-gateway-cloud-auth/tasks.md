Deployment resources, network verification, activation, and image publication belong to separate work.

## 1. Define contracts and authentication boundary

- [x] 1.1 Recheck `test/conformance/upstream.yaml`, the pinned Gateway client source, and existing command evidence; retain findings in `design.md`.
- [x] 1.2 Add and strictly validate the `ai-gateway-cloud-auth` proposal, design, tasks, and delta specification.
- [x] 1.3 Add `ai-gateway/docs/cloud-authentication.md` and link it from `ai-gateway/README.md`; distinguish proposed Cloud behavior from current support.
- [x] 1.4 Add middleware regression tests in `internal/auth/auth_test.go` for internal JWT identity, accompanying `Authorization`, no fallback, and authentication-before-body ordering.
- [x] 1.5 Introduce Gateway-owned `RequestAuthenticator` in `internal/auth/auth.go` with an adapter for existing authlib verification and caller conversion.
- [x] 1.6 Inject authentication failure formatting at the middleware boundary; adapt router dependencies without changing the fixed ProviderWire error.
- [x] 1.7 Run focused middleware tests with `GOWORK=off GOFLAGS=-mod=readonly go test ./cmd/grafana-ai-gateway/internal/auth` from `ai-gateway/`.

## 2. Validate Cloud identity

- [x] 2.1 Add table-driven `internal/auth/cloud_gateway_test.go` cases for valid assertions, opaque non-UUID policy IDs, and distinct identity fields.
- [x] 2.2 Add rejection cases for missing, empty, duplicate, case-colliding, coalesced, control-character, whitespace, signed, zero, nondigit, and overflowing assertions.
- [x] 2.3 Implement `internal/auth/cloud_gateway.go` with `exactlyOneHeader` and the pinned `types.CloudNamespaceFormatter`; make no CAP or scope checks.
- [x] 2.4 Extend `Caller` with a typed authentication source and separate private Cloud fields; leave service and acting-user identity absent for Cloud callers.
- [x] 2.5 Reject surviving `Authorization`, `X-Access-Token`, and `X-Grafana-Id` only at the Cloud ProviderWire entry point, before body reads or protected work.
- [x] 2.6 Run focused authentication tests; prove valid Cloud assertions require no CAP token or outgoing authentication request.

## 3. Configure mode-aware construction

- [x] 3.1 Add failing tests in `internal/config/settings_test.go`, `internal/outbound/outbound_test.go`, and `internal/process/process_test.go` for mode-specific validation and construction.
- [x] 3.2 Add `--auth.mode` and `GRAFANA_AI_GATEWAY_AUTH_MODE` in `internal/config/settings.go`; default to `access-token` and reject unknown modes.
- [x] 3.3 Split JWKS and Anthropic construction in `internal/outbound/outbound.go`; preserve existing transport, redirect, timeout, and response-size protections.
- [x] 3.4 Select the request authenticator in `internal/process/process.go`; skip JWKS endpoint validation, client, verifier, and retrieval in Cloud mode.
- [x] 3.5 Reject unsafe verification and nonempty JWKS URLs in Cloud mode; apply JWKS-specific limits only to the JWT path.
- [x] 3.6 Make the overflow-checked minimum write timeout mode-aware; exclude JWKS latency in Cloud mode and preserve internal accounting.
- [x] 3.7 Verify `ResolveProviderSecrets`, `BuildCatalog`, and server-owned provider configuration remain unchanged; prohibit request-controlled client mutation.
- [x] 3.8 Run focused config, outbound, and process tests from `ai-gateway/` with `GOWORK=off GOFLAGS=-mod=readonly`.

## 4. Separate API and operational listeners

- [x] 4.1 Add failing route-separation tests in `internal/service/router_test.go` and dual-server lifecycle tests in `internal/process/process_test.go`.
- [x] 4.2 Add the optional operational address flag and environment binding in `internal/config/settings.go`; require separate Cloud listeners and loopback for every unsafe listener.
- [x] 4.3 Split exact API and operational dispatch in `internal/service/router.go`; preserve the legacy combined handler, method checks, and encoded-path rejection.
- [x] 4.4 Bind all listeners before readiness in `internal/process/process.go`; close partial startup and stop both servers on either serving failure.
- [x] 4.5 Preserve readiness withdrawal before cancellation, one shared shutdown deadline, forced closure, and cleanup of both serving goroutines.
- [x] 4.6 Add fixed authentication source/outcome observations in `internal/service/telemetry.go`; retain one registry and existing HTTP metrics without private labels.
- [x] 4.7 Add `internal/service/stream_test.go` regressions for `responseWriter.Unwrap`, incremental flushing, and cancellation through the new composition.
- [x] 4.8 Run focused config, listener, lifecycle, and streaming tests with the race detector.

## 5. Prove application behavior

- [x] 5.1 Extend `ai-gateway/test/providerwire-v4/gateway-command.test.ts` with a real-command scenario matrix separating edge denials, overwritten client assertions, and malformed post-edge assertions.
- [x] 5.2 Add a test-only edge shim with fixed dummy credentials and read/write outcomes; remove Cloud/internal credentials, replace identity headers, and forward unchanged paths.
- [x] 5.3 Test discovery with read scope and unary/streaming with write scope using the registered client; supply explicit `maxOutputTokens` for `doGenerate` and `doStream`.
- [x] 5.4 Test invalid dummy credentials, read-only inference, and write-only discovery; assert zero application and provider calls without claiming application error formatting.
- [x] 5.5 Test client assertions overwritten before forwarding and malformed assertions injected after replacement; assert application failures precede body reads and provider work.
- [x] 5.6 Add internal JWT regression with accompanying `Authorization`, API/operational separation, startup failure, flushing, cancellation, and dual-server shutdown scenarios.
- [x] 5.7 Assert fake Anthropic receives only configured provider credentials, never CAP credentials, internal JWTs, identity assertions, or incoming provider keys.
- [x] 5.8 Capture application authentication responses, logs, and metrics; assert fixed ProviderWire errors and no credentials or customer identifiers.
- [x] 5.9 Update `test/conformance/PARITY.md` with command coverage and high-level `generateText`/`streamText` and default unary-token gaps; do not rewrite client requests or provider fixture inputs.
- [x] 5.10 Run `go test ./...` from the repository root.
- [x] 5.11 Run `GOWORK=off GOFLAGS=-mod=readonly go test -race ./...` and `GOWORK=off GOFLAGS=-mod=readonly go vet ./...` from `ai-gateway/`.
- [x] 5.12 Run `mise run test-ai-gateway-command`, `mise run test-providerwire-v4`, and `mise run test-integration` from the repository root.
- [x] 5.13 Run `mise run verify-ai-gateway-boundary` and `mise run verify-module-resolution` from the repository root.
- [x] 5.14 Run `mise run lint`, `mise run parity-check`, and `openspec validate --all --strict` from the repository root.
- [x] 5.15 Run the self-review workflow against the complete application diff and resolve findings.
- [x] 5.16 Run the AI SDK parity review workflow against the registered baseline; report residual client gaps and the limits of shim evidence.
- [x] 5.17 Only after implementation and verification, sync the completed delta to `openspec/specs/ai-gateway-cloud-authentication/spec.md`.
