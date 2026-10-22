# Tasks

Each group is one self-contained commit that keeps `cd providers/grafana && go test -race ./...` passing.

## 1. Remove the raw-usage cap

- [x] 1.1 Remove `maxRawUsageBytes` and its size check from `wireUsage.UnmarshalJSON` in `providers/grafana/model.go`, keeping the empty and non-object rejection. Verify with `go build ./...` in `providers/grafana`.
- [x] 1.2 Replace `TestModel_UnaryRawUsageSize` (`model_test.go`) and `TestModel_StreamRawUsageSize` (`stream_test.go`) with cases that accept and retain a valid `usage.raw` object above 1 MiB on both paths. Keep the whitespace-compaction cases as "retained compacted" without a cap. Verify that both tests pass.

## 2. Make streams unbounded

- [ ] 2.1 In `providers/grafana/stream.go`, drop `limits`, `total`, `count` and the per-event counter from `eventReader`, remove `errStreamLimit` and the `io.LimitReader` wrapper, and drop the `limits` parameter from `consumeStream`. Every non-EOF read error becomes the retryable `stream read failed` part. Update the `DoStream` call site in `model.go`. Verify with `go vet ./...`.
- [ ] 2.2 Replace `TestModel_StreamBounds` with generated-reader cases: more than 100,000 small events, and 65 events above 1 MiB each (single event above 1 MiB plus cumulative above 64 MiB). Assert the delivered part count, no `PartError` and body closure. Add a case asserting that a mid-stream read error yields one retryable status-200 `PartError`. Verify that the tests pass under `-race`.
- [ ] 2.3 Remove the stream-limit setup from `TestGeneratedFileHostileBoundsAndBackpressure` and `metadata_test.go`, keeping their backpressure and malformed-metadata assertions. Drop the case that relied on the 1024-byte event limit (`strings.Repeat(" ", 1025) + "{}"`) or move it to the unary seam. Verify that `TestModel_StreamCancellationAndTransport`, `TestModel_StreamAggregateGoroutineCleanup` and the generated-file and metadata tests pass.

## 3. Introduce the buffered-read backstop

- [ ] 3.1 Add the unexported `maxBufferedResponseBytes` (2 GiB) constant and the `Provider.bufferedReadLimit` field, set in `newProvider`. Route discovery (`discovery.go`), unary success and the over-limit precedence check (`model.go`), and all `readGatewayError` call sites through it, keeping the error messages and precedence. Verify with `go build ./...`.
- [ ] 3.2 Move the over-limit tests to the seam by setting `p.bufferedReadLimit` on the test provider:
  - `TestModel_UnaryReadPrecedence`, `TestModel_UnaryBoundsAndTransport` and the byte-limit case of `TestModel_UnaryProtocolFailures`;
  - `TestListModels_BoundsAndCleanup`, including the error-body case and a non-2xx stream setup case;
  - the unary part of `TestGeneratedFileHostileBoundsAndBackpressure`;
  - `TestListModels_LargeCatalogReadAllowance`, which already overrides `p.limits.DiscoveryBytes`.

  Add a test asserting that `newProvider` sets `bufferedReadLimit` to `2 << 30`. Verify that the exact size passes, one byte over fails non-retryably with no partial result, and the tests pass.

## 4. Remove the public limits API

- [ ] 4.1 Delete `Limits`, `DefaultLimits`, the `Limits` fields on `TokenExchangeConfig`, `ServiceAccountConfig` and `AccessTokenConfig`, the `limits` parameter of `newProvider` and its validation, the `Provider.limits` field and the `math` import in `providers/grafana/provider.go`. Verify with `go build ./...`.
- [ ] 4.2 Update the test helpers and call sites:
  - `testProvider` and `streamFromBody` drop their `*Limits` parameter;
  - remove the invalid-limit cases in `provider_test.go`;
  - remove the remaining `DefaultLimits()` uses in `provider_test.go` and `discovery_test.go`, including the `DiscoveryBytes` default assertion.

  Verify with `go vet ./...` and the tests.
- [ ] 4.3 Extend `public_surface_test.go` to assert by name that there is no exported `Limits` type, no `DefaultLimits` function and no `Limits` field on the three config structs. Verify that the test passes and fails when a `Limits` field is temporarily reintroduced.

## 5. Update godoc and guide

- [ ] 5.1 Update `providers/grafana/doc.go`: drop "bounded" from the streaming sentence and state that the Gateway owns resource bounds and callers bound requests through `http.Client` and `context`. Verify with `go doc ./providers/grafana`.
- [ ] 5.2 Update `docs/providers/grafana-gateway.md`:
  - the raw-output note keeps only the Gateway frame limit;
  - "Handle large catalogs" removes the Go `DiscoveryBytes` advice and keeps the TypeScript helper note;
  - generated files count toward the Gateway's limits;
  - "Bound work and handle errors" replaces the `DefaultLimits` paragraph with server-owned bounds plus `http.Client` and `context`.

  Verify that `rg -n 'Limits|DefaultLimits' docs providers/grafana` has no client-limit hits and `mise run lint-docs` passes.

## 6. Integration checks

- [ ] 6.1 Run `cd providers/grafana && go test -race ./...`, `mise run test-grafana` and `mise run lint` for the module, and verify they pass.
- [ ] 6.2 Run `mise run validate-parity-baseline` and confirm `test/conformance/upstream.yaml` has no new `knownGaps` entry and `PARITY.md` needs no change. Verify that it passes.
- [ ] 6.3 Run `openspec validate remove-grafana-client-limits --strict` and confirm that the change is valid.

## Workflow follow-up

- Open a draft PR titled `refactor(grafana)!: remove client response limits` against `nrbrd/app-platform` (#428), and retarget it to `main` after #428 merges. The description records the backstop and unbounded streams as parity-preserving with upstream `@ai-sdk/gateway@4.0.103`.
- Archive the change in the same PR before merge, and verify the archived `grafana-gateway-client` spec.
