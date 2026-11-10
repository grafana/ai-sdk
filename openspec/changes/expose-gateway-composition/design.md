# Design

## Context

- `ai-gateway/pkg/main.go` builds the plugin logger, calls `gateway.New` from `pkg/internal/gateway`, logs lifecycle events, then calls `plugin.Run` with `gateway.PluginID` and `gw.Authenticator()`, and finally `gw.Close()`. All of it is unreachable from another module.
- The kubeconfig is only available inside the app provider callback (`Gateway.Provider`), which hands it to `simple.NewApp`. #765 needs it there for a namespace config reader.
- Every logical model is `observers(identity(lower))`, built by whoever builds the model:
  - catalog entries once at startup (`service.BuildCatalog` with the configured factory);
  - routed chains per request (`Catalog.selectRouted` with the same factory);
  - BYOK chains per request (`selectBYOK` with the BYOK factory, whose identity is `lower.Provider()` and the requested selector).
- `service.NewModelObservabilityFactories` builds the shared observer middleware once and returns the two factories. The two chains differ only in the log redactor's access policy; metrics bucket BYOK by `provider != "grafana"`.
- `execution.Runtime.Call` runs the selector inside `selectModel`, which contains panics and validates the selection before any model call.
- Go's internal rule: `pkg/internal/...` is importable only below `ai-gateway/pkg/`.
- Module policy: Go source outside `ai-gateway/` must not import the Gateway, and any tracked `go.mod` under `ai-gateway/` is a published module that `internal/releasecheck` requires to be registered with release-please.

## Goals / Non-Goals

**Goals:**

- One public entrypoint a distribution's `main` can call without missing plugin ID, authenticator, kubeconfig handoff or lifecycle.
- One seam, after selection, where per-call behavior applies to every selection.
- A selection value that later selection types (#757) and the extension pipeline (#762) extend without restructuring.

**Non-Goals:**

- Extensions, an `Extensions` option, `DefaultExtensions()`, settings or kinds (#762, #764).
- Stack-catalog selection, request-scoped redaction, catalog identity, a third observer chain (#757).
- A kubeconfig consumer (#765). Changes to the app provider wiring beyond moving its caller (#756).

## Decisions

### Package path: `github.com/grafana/ai-sdk/ai-gateway/pkg/gateway`

The entrypoint must import `pkg/internal/gateway`, so the internal rule forces it below `ai-gateway/pkg/`. A module-root package next to `catalog` and `providerwire` would require moving every `pkg/internal` package, which conflicts with leaving existing tests unchanged. The public package imports the internal one under an alias; both keep the name `gateway`.

### API: `func Run(Options) error` with an empty `Options` struct

```go
type Options struct{}

func Run(Options) error
```

- `Run` is today's `main.run`: it builds the plugin logger, reads `os.LookupEnv`, registers with `prometheus.DefaultRegisterer`, calls the internal `New`, logs `plugin_idle` or `plugin_started`, calls `plugin.Run(gw.Provider(), plugin.WithPluginID(PluginID), plugin.WithAuthenticator(gw.Authenticator()))`, closes the Gateway and logs `plugin_stopped`.
- It logs failures itself with today's classes and returns the error, so `main` is `if gateway.Run(gateway.Options{}) != nil { os.Exit(1) }`. Returning `error` instead of an exit code keeps Go convention.
- The kubeconfig handoff is `gw.Provider()`'s callback; `Run` is its only caller, so a distribution cannot bypass it. #765 adds its reader in that callback.
- `Options` carries no fields yet. `Extensions` needs an element type that only #762 defines; inventing one now would fix an API before its design. A zero-value struct reserves the parameter so #762 adds a field without changing `Run`'s signature.
- `pkg/native_proxy_trust.go` (build tag `gatewaytest`) stays in package `main`; distributions never get the test CA hook.
- Alternative considered: functional options (`Run(...Option)`). Rejected because the issue and the later `Options.Extensions` composition (`append(gateway.DefaultExtensions(), private...)`) are struct-shaped.

### Seam: an optional model composer in `execution.Config`, applied in `selectModel`

```go
type ModelComposer func(selected Selection) (provider.LanguageModel, error)

type Config struct {
	Selector RequestSelector
	Compose  ModelComposer
	...
}
```

- `selectModel` validates the selection, then replaces `Model` with `Compose(selected)` and validates the result, under the same panic containment. A composer error, panic or invalid model becomes `errInternal`, so the call fails as internal with zero provider requests. A nil composer leaves the selection unchanged, which keeps existing runtime tests valid.
- The name describes the role, not the mechanism: the composer builds the model one call invokes from its selection, so it still fits when #762 adds the extension pipeline at the same point.
- The composer receives the whole `Selection`, so #757 can bind catalog identity into the per-call chain and #762 can read source and planned candidates at the same point.
- Alternative considered: a `RequestSelector` decorator in `service`. It works for observers, but #762 must hook the runtime anyway (a denial before the model is entered must not synthesize an attempt), and a decorator is a second composition step that can be applied twice or skipped.

### Selection source

```go
type Source string

const (
	SourceOperatorCatalog Source = "operator-catalog"
	SourceRequestBYOK     Source = "request-byok"
)
```

- `Selection` gains an exported `Source` field. `CatalogSelector` and `ConfiguredSelection` set `operator-catalog`; `AccountSelection` sets `request-byok`. Constructor signatures stay the same. #757 sets `stack-catalog` on a configured-attribution selection.
- Source is separate from the overview attribution: a stack-catalog selection will project like a configured one but be observed differently.
- Key ownership is not stored: it is a function of source (`operator-catalog` is `operator`; `request-byok` and `stack-catalog` are `customer`). #762 adds it where extensions filter on it.

#### Fields deferred

| Field | Owner | Reason |
|---|---|---|
| Request-scoped keys for redaction | #757 | Consumed only by request-scoped summary redaction, which #757 owns. BYOK keys are deliberately not redacted from caller-visible summaries today. |
| Catalog identity (namespace, name, resource version) | #757 | Consumed only by the stack-catalog log and metadata policy. |
| Planned candidates | #762 | Already carried as the unexported expected attempts; #762 exposes them to `Admit`. |
| Key ownership | #762 | Derived from source; no consumer yet. |

### Observers: `service.ModelObservers`

- `NewModelObservers(telemetry, logger, runtime, drainTimeout)` builds what `NewModelObservabilityFactories` builds today: one metrics registration, two logger middlewares (configured and BYOK redaction policies), the context bridge and the optional Agent Observability recorder.
- Its `Compose(selection)` is the runtime's `ModelComposer`: it picks the chain by `selection.Source` and returns `middleware.Wrap({Model: selection.Model, Middleware: chain})`; an unknown source returns an error. Wrapping per call allocates one wrapper per observer and constructs no middleware or collectors.
- Identity stays where the model is built, since it depends on how the model was selected:
  - `service.ConfiguredIdentity` (provider `grafana`, model the canonical ID) for catalog entries and routed chains; it is today's `identityModelFactory`, which stays as the implementation because existing tests call it;
  - `service.BYOKIdentity(id, lower)` (provider `lower.Provider()`, model the requested selector) for BYOK.
- `BuildCatalog`, `Catalog.selectRouted` and `BYOKOptions.Factory` keep their `ModelFactory` parameters; production passes the identity factories instead of the observing ones.
- `NewModelObservabilityFactories` has no production caller afterwards. It moves to a new `_test.go` helper composed of `ConfiguredIdentity`/`BYOKIdentity` and `ModelObservers.Compose`, so the existing tests that build observed models directly keep compiling unchanged and keep exercising production observers. The same helper file keeps the two other names existing tests use: `identityModelFactory`, as an alias of `ConfiguredIdentity`, and the chain's `wrap` method.

### Production wiring

`pkg/internal/gateway.newRouteHandler` builds `ModelObservers`, passes `ConfiguredIdentity` to `BuildCatalog`, `BYOKIdentity` to `BYOKOptions`, and `observers.Compose` to `execution.Config.Compose`. Discovery is unaffected: it lists model info only.

### Compile test from an external module

`pkg/gateway` gets a test that generates a throwaway module `example.com/distribution` in a temporary directory, with a `main` that calls `gateway.Run(gateway.Options{})`, and builds it with the `go` tool:

- `go.mod` requires `github.com/grafana/ai-sdk/ai-gateway`, copies `ai-gateway/go.mod`'s requirements, and replaces the Gateway and every first-party module with local source (absolute paths derived from `ai-gateway/go.mod`'s replacements); `go.sum` is copied from `ai-gateway/`.
- It builds with `GOWORK=off`, `GOFLAGS=-mod=readonly` and `GOPROXY=off`, so it uses only the module cache the Gateway tests already need. It skips under `-short`, like `buildPlugin`.
- The module path is outside `github.com/grafana/ai-sdk/ai-gateway/`, so the Go tool refuses any Gateway internal import: a nested module under `ai-gateway/` could still import `ai-gateway/internal/...`.
- Alternative considered: a tracked module under `ai-gateway/test/`. `internal/releasecheck` and `module-policy.sh` classify it as published and require release-please registration, so it needs policy exclusions, and it proves less.

## Test plan

| Acceptance | Evidence |
|---|---|
| No behavior change | Existing Go suites unchanged, including plugin-process tests that build `pkg` and assert lifecycle events, health and stop; `mise run test-ai-gateway`, `lint`, `vet`, `build`, `test-integration`, boundary and requirements checks. |
| Compile from an external module | The generated-module build test above. |
| Identical identities | One table-driven service test over the production composition (`ModelObservers`, identity factories, `NewSelector`, `execution.Runtime` with `Compose`) for a configured call, an alias call, a routed call served by a `models` entry and a BYOK call, unary and stream. It asserts each logical observer runs once and the identities: logs and Agent Observability `grafana`/canonical ID (routed: the requested canonical ID), BYOK requested provider/selector; metric `model` label canonical ID or `byok`. |
| Per-request selection double | A test selector that builds a new `ConfiguredSelection` per request around a fake model holding that request's key. Two calls with different keys: each reaches its own model, each is observed once with the operator-catalog chain, and neither key appears in logs, metrics or Agent Observability metadata. An empty source fails as internal with zero model calls. |

## Risks / Trade-offs

- [Per-call wrapping cost] → A handful of small allocations per call; no middleware or collector construction. Accepted.
- [The observing test helper drifts from production] → It is composed only of production pieces; the identity test covers the production wiring itself.
- [Unknown source fails a request] → Fails closed with zero provider requests; every production selector sets a source, enforced by the identity test.
- [#756 edits `Gateway.Provider`] → `Run` only calls it; the callback stays in `pkg/internal/gateway`, so #756 rebases with a `main.go`-only conflict at most.
- [#757 stacks on the seam] → See Follow-ups for #757.
- [#762 public vocabulary] → `Source` lives in `internal/execution`; the public `extension` package needs a public type. #762 moves the type to a public leaf package and execution uses it.
- [Experimental public API] → Documented as v0; the release changelog lists the refactor.

## Follow-ups for #757

- **Metric model label.** The Prometheus model middleware is registered once, and its `NormalizeModel` picks the `byok` bucket from the logical provider (`provider != "grafana"`), not from the selection source. A stack-catalog chain cannot register a second metrics middleware, and stack-catalog models keep provider `grafana`, so their customer model IDs would become labels. #757 needs a normalizer that knows the source: for example a fixed logical provider for stack catalogs, or a context-aware normalizer fed by the composer.
- **Optional operator catalog.** `service.NewSelector` returns nil for a nil catalog, and `execution.New` refuses a nil selector. Serving without `model_catalog` means restructuring the selector; the composer seam is unaffected.
- **Request-scoped redaction and catalog identity.** Add them to `Selection` with their consumers: the summary redactor merges a selection's keys, and the stack-catalog chain reads the catalog identity from the selection the composer receives.
- **Source.** Add `SourceStackCatalog` with its observer chain in `ModelObservers`; until then the composer refuses it, which the unknown-source test covers.
