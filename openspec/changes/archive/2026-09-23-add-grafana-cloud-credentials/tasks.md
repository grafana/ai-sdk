## 1. Establish the regression contract

- [x] 1.1 Reconfirm the registered Gateway/AI package versions and matching upstream auth implementation/tests; record the provider-request/host-composition coverage boundary without changing pins.
- [x] 1.2 Add a deterministic Vercel capture using `apiKey: "<stack-id>:<dummy-cap>"` and failing Go regression coverage for equivalent discovery, unary and streaming Cloud auth. Establish failure before adding CAP support; do not modify recorded provider inputs.
- [x] 1.3 Add failing validation/security cases for invalid stack IDs and tokens, reserved configured/call headers under varied casing, acting-user conflicts, canceled calls, redirects and absence of credential-bearing body metadata.

## 2. Implement explicit client authentication flows

- [x] 2.1 Rename `NewWithCloudAuth`/`CloudAuthConfig` to `NewWithTokenExchange`/`TokenExchangeConfig` without aliases; migrate repository-owned production/test/capture call sites and preserve existing exchange/JWT tests. Leave archived historical artifacts intact.
- [x] 2.2 Add `CloudCredentialsConfig` and `NewWithCloudCredentials` with positive `StackID int64`, opaque CAP validation and the common client/header/limit options; preserve immutable configuration, URL validation and redirect refusal.
- [x] 2.3 Add the private direct-CAP request-auth path for discovery, unary and streaming: exactly one stack/CAP bearer header, no exchanger or exchange gate, and no JWT/identity assertion headers or authentication fallback.
- [x] 2.4 Enforce Cloud reserved-header rejection before body serialization/network I/O and reject nonempty context acting-user tokens. Verify safe errors, cancellation, concurrent use and unchanged non-reserved header projection.

## 3. Prove cross-client and server-boundary behavior

- [x] 3.1 Extend the Go capture harness with explicit Cloud-credentials and renamed token-exchange configuration, including rejection of ambiguous auth selections.
- [x] 3.2 Extend the existing deterministic edge/real-command suite to exercise both Go and pinned Vercel discovery, low-level unary and streaming calls with identical stack/CAP credentials; assert no exchange traffic and unchanged ProviderWire bodies/results.
- [x] 3.3 Verify invalid credentials, read/write scope denials, raw-request spoofed identity replacement, credential stripping and no downstream provider calls after edge rejection. Preserve server-mode isolation and JWT/exchange regression cases.
- [x] 3.4 Assert Cloud credentials do not enter provider requests, client-generated request metadata, application logs or metrics. Review request expectation changes explicitly and update `test/conformance/PARITY.md` only for stable new evidence or accepted stricter header behavior; retain the live-ingress coverage gap.

## 4. Rewrite authentication documentation for Go and Vercel

- [x] 4.1 Create `docs/guides/gateway-authentication.md` with a credential/endpoint compatibility table and a request-flow explanation separating application login, gateway authentication and server-owned provider credentials.
- [x] 4.2 Add working Go `NewWithCloudCredentials` and server-side Vercel `createGateway` examples with the API-prefix URL, catalog discovery and tested generation/streaming usage. State explicitly that neither direct-CAP flow exchanges tokens and that credentials do not belong in browser code.
- [x] 4.3 Document Go token-exchange and pre-minted JWT flows, their access-token-endpoint prerequisite, refresh ownership, optional acting-user support limited to JWT flows, and the constructor/type rename. Explain why a JWT in Vercel `apiKey` is not the same as `X-Access-Token`; do not advertise untested exchange adapters.
- [x] 4.4 Document required read/write scopes, stack versus organization IDs, least-privilege realms, HTTPS, credential storage/expiry/rotation and troubleshooting. Clearly exclude k6 worker migration, system-CAP distribution and unsupported Cloud acting-user delegation.
- [x] 4.5 Rewrite conflicting/duplicated auth sections in `docs/providers/grafana-gateway.md` and the existing Gateway Cloud guide; link the shared guide from `docs/README.md`, those entry points and the container guide. Update godoc for the new/renamed API and correct touched stale baseline references without broad unrelated rewrites.
- [x] 4.6 Compile/typecheck and exercise the documented configurations and operations with deterministic Go/Vercel tests. Run `mise run lint-docs` and check navigation, migration examples and endpoint compatibility claims against the code.

## 5. Validate and prepare the release handoff

- [x] 5.1 Run Grafana-provider tests including the race detector from `providers/grafana`, relevant formatting/vet/lint checks, and `mise run verify-module-resolution` plus `mise run verify-ai-gateway-boundary` to preserve standalone module behavior and the Apache/AGPL boundary.
- [x] 5.2 Run `mise run parity-check` and `mise run test-integration` (which includes the Gateway command suite); record results and any blockers. Confirm no server auth/payload changes or unexplained fixture churn.
- [x] 5.3 Prepare the external handoff: identify affected consumer owners for the breaking rename and separate `deployment_tools` policy/secret configuration changes. Include a real-edge acceptance checklist for both clients, wrong-stack/scope, invalid/expired/revoked credentials, stripping and network isolation. Do not provision, access secrets or deploy as part of this task.
- [x] 5.4 Validate the OpenSpec change and review implementation/docs against every acceptance scenario. Keep live rollout and the k6/IAM delegated-auth decision explicitly separate from locally completed feature work.
