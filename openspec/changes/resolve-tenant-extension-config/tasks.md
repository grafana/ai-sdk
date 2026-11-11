# Tasks

## 1. Test host list

- [x] 1.1 Add namespace list for declared kinds to `test/platformhost` (`GET .../namespaces/<ns>/<plural>`, a `<Kind>List` in name order, existing namespace check). Verify with `kinds_test.go` cases: two namespaces, an empty namespace, and a refused foreign namespace.

## 2. Namespace reader

- [x] 2.1 Add `extensionhost.TenantResource` and `TenantReader`, then `tenantconfig.NewReader(rest.Config)` and `Read` (group and version, generic serializer, explicit client rate limit, paged list, `maxListBytes` bound, raw `spec.config`, compaction, sorted output, classed errors). Make `k8s.io/client-go` a direct requirement. Verify with `httptest` table tests: empty list, two pages, exact `9007199254740993` and `1.0`, absent or `null` config, a missing `enabled` reported per resource, 401, 403, 404, 500, a timeout, malformed JSON and an oversized body, each with its class and no body text in the error.
- [x] 2.2 Verify plugin identity with a real app-sdk token-exchange kubeconfig against the test host's exchange handler: reads of two namespaces succeed and every exchange request has no subject token, while a read context carrying a caller would be delegated (which is why the cache never passes one). The test host accepts the access token repeated as a bearer token, as the app-sdk transport sends it.

## 3. Shared registerer and contract docs

- [x] 3.1 Add the per-definition `sharedRegisterer` and pass it to every instance. Update the godoc of `Definition.New`, `Dependencies.Registerer` and `ScopeTenant` for per-namespace construction and the collector contract. Verify: re-registering the same collector succeeds, a different collector with the same descriptors fails, an uncomparable collector type doesn't panic, and gather shows one series for two registrations.

## 4. Tenant cache and snapshots

- [x] 4.1 Add `TenantCacheConfig` with the named constants and the cache: LRU entries, acquire, deduplicated first load, background refresh, fixed backoff, last-known-good bound, failure snapshot, a detached root context and read timeout. Verify with `synctest` tests: one read for ten concurrent first acquires; a caller cancelling while the read continues and is reused; reads at `ttl-1ns` and `ttl`; no read before `retryAt`; last-known-good at `+10m-1ns` and the failure snapshot at `+10m`; eviction at `maxNamespaces`; and the reader context carrying no `AuthInfo`.
- [x] 4.2 Add snapshot building: the per-definition state table, `CheckConfig` on compacted configuration, contained factories, unknown types logged and counted once per entry and type, disabled unknown types ignored, the digest for unchanged refreshes, and the lists and unavailable definitions per source. Add `tenant_config_reads_total{result}`, `tenant_config_unknown_types_total` and `extension_events_total{phase="configure"}` with their warnings. Verify with table tests for every row (including `name != type` and a schema-less `{ }`), no factory run on an unchanged refresh, sentinel-free logs, and metric label sets.

## 5. Pipeline and lifecycle

- [x] 5.1 Resolve lists at invocation: `tenantSources`, acquiring only when a tenant definition applies, a refusal before admit for unavailable definitions with an observe record, and release points for unary, a direct stream and observed calls. Verify with pipeline tests: isolation across four namespaces (configured, other config, absent, disabled default-on), unary and stream; first-read failure under `skip` and `fail-closed` with zero model calls; a change applied after a refresh under `synctest`; zero reads for operator-only and narrowed sources.
- [x] 5.2 Add reference counting, retire on replacement, eviction, expiry and evicted results, bounded contained close, and `Host.Close` covering snapshot instances. Verify under `-race`: replacement during an open stream closes the old instance once, after the stream and its observation; eviction with `maxNamespaces` 1 closes after release; and shutdown closes live and retiring snapshot instances within the budget.
- [x] 5.3 Verify the shared-collector acceptance: a counter registered by two namespaces' factories yields one series, and a fresh collector on each run applies the failure policy for the second namespace only.

## 6. Gateway wiring

- [x] 6.1 In `Gateway.Provider()`'s callback: decide API access from the kubeconfig; build the reader and `ConnectTenants` when tenant definitions exist; log the no-access warning once; emit the API-access Info record; and build the managed kinds with `tenantconfig.NewValidator(definitions, apiAccess)`. Add the no-access refusal before every other admission check. Verify `tenantconfig` validator tests (refusal precedes disable-only, fixed reason) and `TestProvider_ManagesTheManifestKinds` for running and idle Gateways.
- [x] 6.2 Add gateway tests through the test host with test-only definitions and a token-exchange kubeconfig pointing at it:
  - resources created via admission in two namespaces apply to their own language-model calls;
  - reads use the plugin token (no subject token in the exchange log);
  - the shipped definitions make no list request;
  - with a zero kubeconfig: no list request, defaults applied, one warning, creates refused with `400`, and deletes unreviewed.

  Verify they pass.

## 7. End-to-end suite

- [x] 7.1 Add the positive plugin-log check to `scripts/e2e.sh`: the API-access record is present and reports access, before the existing sentinel checks. Verify the script passes `shellcheck` if available and runs in CI.

## 8. Documentation

- [ ] 8.1 Update `docs/guides/gateway-extensions.md` for applied configuration:
  - the runtime state table, TTL propagation and replica convergence;
  - read failures and the 10-minute bound;
  - the no-API-access refusal and log;
  - per-namespace factories and the collector idiom;
  - `skip` guidance for tenant gates;
  - that tenant extensions see the caller UID and the provider instances and models of operator-catalog candidates and attempts.

  Replace "not applied yet". Update the architecture walkthrough (namespace configuration path) and glossary (tenant configuration cache, last-known-good). Verify `mise run lint-docs` passes.

## 9. Integration checks

- [ ] 9.1 Run `mise run fmt-check`, `vet`, `lint`, `build`, `test-ai-gateway` (with `-race` on `extensionhost` and `tenantconfig`), `test-integration` and `verify-ai-gateway-boundary` with `GOFLAGS=-buildvcs=false`. Verify they pass, and record the CI-only checks: plugin build and validation, end-to-end suite, `module-policy.sh`, parity coverage.
- [ ] 9.2 Run `openspec validate resolve-tenant-extension-config --strict` and verify it passes.

## Workflow follow-up

- Manual development deployment check for nara on the first build with a tenant extension (#766): see design.md, "Development deployment check (manual)".
- Open a follow-up issue for admission warnings through app-sdk (option A), linked from the guide.
- Archive the change in the same PR after review.
