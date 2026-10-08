# Validation

## Scope

Shared fallback observation only. Request-local capture uses a context callback and adds no model-global history, collector, wire DTO or metadata serialization. The existing dormant Gateway evidence implementation is unchanged; its compact replacement and runtime activation are not established by this stage.

Reference: ai 7.0.118/provider 4.0.18 at `5d12eaa6caa193d3901cbab98a734403eb6bf622`. The pinned APICallError source preserves native data separately from causes; no corresponding upstream fallback implementation exists. Observation is a Go adaptation. No provider fixture inputs or recordings changed.

## Regression evidence

- Tests first failed to build because request-scoped observation and `SourceErr` were absent (`/tmp/373-overview-design/sdk-red.log`).
- Unary/stream observers receive the same ordered decision snapshots; either callback can panic without suppressing the other or changing selection.
- Child registrations replace or disable inherited request callbacks independently of model observation.
- Twenty-four independent request scopes share a fallback model under the race detector without callback leakage.
- Cancellation/deadline during the decider retains original candidate failures in `SourceErr` while decision/returned errors preserve the context cause.
- Accepted leading error parts remain selected ordered stream content, with no setup source error.
- Setup/validation/EOF source errors and exclusion of unowned late native setup failures retain existing commitment and cleanup semantics.
- GenerateText/StreamText expose earlier candidate failures through request observation while selected provider metadata is unchanged and a panicking operator observer is isolated.

## Passed commands

- `go test -race ./...` and `go vet ./...` with readonly module dependencies.
- `golangci-lint run --build-tags conformance ./...`: zero issues.
- Full root race tests on Go 1.26.8 with readonly dependencies.
- Full Gateway source-workspace tests and focused evidence/ProviderWire/service races.
- `mise run parity-check`: registered type/schema/client/runtime and provider replays.
- `mise run lint-docs`: structural and markdown checks.
- Gateway workspace, module/license boundary, SDK isolation and merged-pin gates.
- `openspec validate --all --strict`: 94 items, zero failures.
- `git diff --check`; unrelated `go.work.sum` patch compared byte-for-byte with the initial patch and preserved.

Logs: `/tmp/373-overview-design/sdk-{root-race,gateway,go126,parity,boundaries}.log`.

## Remaining work

No fresh independent review is claimed. Gateway reduction remains a separate stage in `collect-gateway-execution-evidence`, including an explicit policy for optional enrichment when native namespace relocation alone exceeds the existing envelope limit. No production delivery, both-client overview access or live-provider proof is claimed here. OpenSpec synchronization/archive remains pending owner approval.
