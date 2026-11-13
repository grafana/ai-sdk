# Tasks

## 1. Test host list

- [x] 1.1 Add namespace list for declared kinds to `test/platformhost` (`GET .../namespaces/<ns>/<plural>`, a `<Kind>List` in name order, existing namespace check). Verify with `kinds_test.go` cases: two namespaces, an empty namespace, and a refused foreign namespace.

## 2. Namespace reader

- [x] 2.1 Add `extensionhost.TenantResource` and `TenantReader`, then `tenantconfig.NewReader(rest.Config)` and `Read` (group and version, generic serializer, explicit client rate limit, paged list, `maxListBytes` bound, the type's member decoded with `DecodeSpec`, invalid specs marked per resource, sorted output, classed errors). Make `k8s.io/client-go` a direct requirement. Verify with `httptest` table tests: empty list, two pages, nested members with the largest safe integer, no member, another type's member, two members and a missing `enabled` reported per resource, 401, 403, 404, 500, a timeout, malformed JSON and an oversized body, each with its class and no body text in the error.
- [x] 2.2 Verify plugin identity with a real app-sdk token-exchange kubeconfig against the test host's exchange handler: reads of two namespaces succeed and every exchange request has no subject token, while a read context carrying a caller would be delegated (which is why the cache never passes one). The test host accepts the access token repeated as a bearer token, as the app-sdk transport sends it.

## 3. One instance per extension and contract docs

- [x] 3.1 Construct every extension once per process: plan every tenant definition with an empty configuration, decode the empty configuration of default-enabled types once at startup, and refuse a decoder on an operator definition and secrets on a tenant one. Add `Definition.Decode`, `tenanttype.Implementation.Decode` and `Call.Config`, and update the godoc of `Definition.New`, `Dependencies.Registerer` and the package. Verify: every tenant factory runs once with `{}`, the default configuration is decoded once and a failing one fails startup, a phaseless tenant instance fails startup and is closed, and the registration refusals.

## 4. Tenant cache and namespace configuration

- [x] 4.1 Add `TenantCacheConfig` with the named constants and the cache: LRU entries, acquire, deduplicated first load, background refresh, fixed backoff, last-known-good bound, failure configuration, a detached root context and read timeout. Verify with `synctest` tests: one read for ten concurrent first acquires; a caller cancelling while the read continues and is reused; reads at `ttl-1ns` and `ttl`; no read before `retryAt`; last-known-good at `+10m-1ns` and the failure configuration at `+10m`; eviction at `maxNamespaces`; and the reader context carrying no `AuthInfo`.
- [x] 4.2 Add namespace configuration building: the per-definition state table, `CheckConfig` on the type's member, contained decoding, types not implemented logged and counted once per entry and type, disabled ones ignored, the digest for unchanged refreshes, and the lists and unavailable definitions per key ownership. Add `tenant_config_reads_total{result}`, `tenant_config_unimplemented_types_total` and `extension_events_total{phase="configure"}` with their warnings. Verify with table tests for every row (including `name != type`, a missing member and a member for a schema-less type), no decoding on an unchanged refresh, sentinel-free logs, and metric label sets.

## 5. Pipeline and configuration per call

- [x] 5.1 Resolve lists at invocation: `tenantKeys`, acquiring only when a tenant definition applies, a refusal before admit for unavailable definitions with an observe record, and each view's `Call.Config`. Verify with pipeline tests: isolation across four namespaces (configured, other config, absent, disabled default-on), unary and stream; first-read failure under `skip` and `fail-closed` with zero model calls; a change applied after a refresh under `synctest`; zero reads for operator-only extensions and a key ownership no tenant extension applies to.
- [x] 5.2 Make namespace configurations immutable and swap them under the cache lock, with no reference counts or per-namespace close, and close instances once at `Host.Close`. Verify under `-race`: a stream open across a refresh and an eviction keeps its configuration and nothing is closed until shutdown, which closes the instance once; a typed `Decode` result is shared by a namespace's calls, raw JSON without `Decode`, nil for operators; `Decode` runs once per change; a collector registered in `New` counts calls from two namespaces.

## 6. Gateway wiring

- [x] 6.1 In `Gateway.Provider()`'s callback: decide API access from the kubeconfig; build the reader and `ConnectTenants` when tenant definitions exist; log the no-access warning once; emit the API-access Info record; and build the managed kinds with `tenantconfig.NewValidator(definitions)`, or `tenantconfig.WithoutAPIAccess`, which refuses every create and update, when there is no access. Verify `tenantconfig` validator tests (refusal precedes disable-only, fixed reason) and `TestProvider_ManagesTheManifestKinds` for running and idle Gateways.
- [x] 6.2 Add gateway tests through the test host with test-only definitions and a token-exchange kubeconfig pointing at it:
  - resources created via admission in two namespaces apply to their own language-model calls;
  - reads use the plugin token (no subject token in the exchange log);
  - the shipped definitions make no list request;
  - with a zero kubeconfig: no list request, defaults applied, one warning, creates refused with `400`, and deletes unreviewed.

  Verify they pass.

## 7. End-to-end suite

- [x] 7.1 Add the positive plugin-log check to `scripts/e2e.sh`: the API-access record is present and reports access, before the existing sentinel checks. Verify the script passes `shellcheck` if available and runs in CI.

## 8. Documentation

- [x] 8.1 Update `docs/guides/gateway-extensions.md` for applied configuration:
  - the runtime state table, TTL propagation and replica convergence;
  - read failures and the 10-minute bound;
  - the no-API-access refusal and log;
  - one instance per extension, `Call.Config` and `Decode`;
  - `skip` guidance for tenant gates;
  - that tenant extensions see the caller UID and the provider instances and models of operator-catalog candidates and attempts.

  Replace "not applied yet". Update the architecture walkthrough (namespace configuration path) and glossary (tenant configuration cache, last-known-good). Verify `mise run lint-docs` passes.

## 9. Integration checks

- [x] 9.1 Run `mise run fmt-check`, `vet`, `lint`, `build`, `test-ai-gateway` (with `-race` on `extensionhost` and `tenantconfig`), `test-integration` and `verify-ai-gateway-boundary` with `GOFLAGS=-buildvcs=false`. Verify they pass, and record the CI-only checks: plugin build and validation, end-to-end suite, `module-policy.sh`, parity coverage.
- [x] 9.2 Run `openspec validate resolve-tenant-extension-config --strict` and verify it passes.

## Workflow follow-up

- Manual development deployment check for nara on the first build with a tenant extension (#766): see design.md, "Development deployment check (manual)".
- Open a follow-up issue for admission warnings through app-sdk (option A), linked from the guide.
- Archive the change in the same PR after review.
