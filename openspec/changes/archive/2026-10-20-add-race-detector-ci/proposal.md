## Why

Issue #419 makes stream worker goroutines recover from panics. A data race such as a concurrent map write is fatal in Go and cannot be recovered, so the stream-path modules need the race detector in CI. No CI job runs it today.

## What Changes

- Add a `test-race` mise task that runs `go test -race -short` for the root module, the Gateway (through `go.gateway.work`), the anthropic, openai and openai-compatible providers, and the logger, prometheus and agentobservability middleware.
- Run it in a separate `race` job so the `ci` job's wall time does not grow, with its own Go build cache key.
- Gateway image publication requires the new job, like the other source checks, and the workflow structural test asserts both.

## Capabilities

### Modified Capabilities
- `candidate-source-ci`: Required source CI also runs the race detector on the Gateway and its stream-path modules.

## Impact

CI configuration, `mise.toml`, the workflow structural test and one line in `CONTRIBUTING.md`. No runtime behavior changes. Run locally on main, the task finds no data races and takes about 70 seconds.
