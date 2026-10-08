# Tools

Tools let a model request actions or information from your application. Use them
when a response depends on live data, deterministic computation, or a side
effect that ordinary text generation cannot perform safely.

## Start with a typed tool

`TypedTool` derives the input schema from a Go type and converts inputs and
outputs for you:

```go
type WeatherInput struct {
	City string `json:"city" jsonschema:"description=City to look up"`
}

type WeatherOutput struct {
	TemperatureC int `json:"temperatureC"`
}

weather, err := aisdk.TypedTool(aisdk.TypedToolDef[WeatherInput, WeatherOutput]{
	Name:        "get_weather",
	Description: "Get the current weather for a city.",
	Execute: func(ctx context.Context, input WeatherInput, _ aisdk.ToolExecutionOptions) (WeatherOutput, error) {
		return lookupWeather(ctx, input.City)
	},
})
if err != nil {
	return err
}

result := aisdk.StreamText(ctx, model,
	aisdk.WithModelMessages(provider.UserText("What is the weather in Paris?")),
	aisdk.WithTools(aisdk.ToolSet{"get_weather": weather}),
	aisdk.WithStopWhen(aisdk.StepCountIs(5)),
)
```

Tool names and descriptions help the model choose correctly. Input schema
descriptions and constraints should explain valid values and their business
meaning.

Run [`examples/agent-chat`](../../examples/agent-chat) for a complete agent
that executes a typed tool across model steps and streams the result to
`useChat`.

## Decide where execution happens

A tool with `Execute` or `ExecuteStream` runs in the Go process. The SDK returns
its final output to the model and can continue to another step. Streaming tools
can emit preliminary outputs to the UI before completing; each emitted value is
preliminary, and the last is repeated as the final result. Only the final
result enters the next model prompt. A tool must not configure both execution
forms. `TypedTool` currently wraps single-result executors; configure a `Tool`
directly for streamed execution.

A tool without either execution function is external. Its call is emitted to the result stream
and the current loop stops so a browser, queue worker, or another service can
handle it. Resume with the resulting conversation state after the external
action completes.

Some providers also supply provider-executed tools such as web search or code
execution. Those are configured through the provider package and run by the
provider, not by your `Execute` function. A provider-defined tool may also
return a call for your application to execute. Check `ProviderExecuted` on each
call to determine who owns execution.

## Route tools through callers

Use caller routing when the model should call a local or provider caller instead
of seeing every application tool directly. Configure the caller tool in the
`ToolSet`, then use `WithToolRoutes` to map each **callee** to its caller tool
names:

```go
aisdk.WithToolRoutes(aisdk.ToolRoutes{
    "get_inventory": {Callers: []string{"code_mode"}},
    "get_weather":   {Direct: true, Callers: []string{"code_mode"}},
    "internal":      {},
})
```

A callee routed only through a local caller is omitted from the provider's
direct tool list but remains in that caller's bound tool set. `Direct` also
exposes the callee to the model; a zero-value route hides it, while an unlisted
tool retains its ordinary behavior. Active-tool settings are applied per step
before binding callers.

A provider caller can prepare the routed tool's provider options, such as
provider-specific `allowedCallers`. Manually supplied provider options pass
through when no caller preparation is configured; otherwise the preparation
callback controls the resulting options. Different providers use different
caller names and semantics. Caller routing mirrors upstream exposure and
binding; it does **not** independently authorize an unexpected provider-emitted
call against caller metadata. Enforce any security policy required by your
application within the tool's executor and approval policy.

## Discover a deferred registry

For a large registry, opt into core keyword discovery rather than advertise every
schema on the first step. A typed tool can be configured after construction:

```go
weather.DeferLoading = true
result := aisdk.StreamText(ctx, model,
    aisdk.WithModelMessages(provider.UserText("Find the weather in Paris.")),
    aisdk.WithTools(aisdk.ToolSet{
        "search": aisdk.ToolSearch(),
        "getWeather": weather,
    }),
    aisdk.WithStopWhen(aisdk.StepCountIs(5)),
)
```

Search returns up to five matching names and descriptions, not schemas. A match
becomes available only on the **next** model step. The default one-step limit is
unchanged, so configure a multi-step stop condition. Undiscovered tools are absent
from both model definitions and new-call execution bindings; a premature call
returns a tool error. Discovery is isolated to each generation, even when calls
share a registry or agent. Active-tool selection still limits discovery and later
availability; an explicitly empty selection disables all step tools.

Names score above descriptions, repeated query terms count once, and equal scores
use sorted tool names because Go maps have no insertion order. This is keyword
search, not semantic retrieval or an authorization policy.

By default, search discovers directly callable entries. To discover through a
local caller, route search and deferred callees to the same active caller using
`ToolRoutes`. That caller must supply `PrepareModelMessage`, so its updated catalog
is announced on the next step while its model definition remains stable. A
same-step nested invocation cannot use newly discovered tools. Provider callers
and local callers without announcements are unsupported for deferred discovery.
A zero-value route has no shared discovery route. Core search never changes a
provider-defined tool into a local function and is separate from provider-hosted
search.

For descriptions that depend on request context, set `DescriptionFunc`; it
receives the effective step runtime context and overrides the static description,
including an empty result. Set context through `PrepareStep` or agent runtime
context options. Search resolves candidates at execution; later model definitions
and caller catalogs resolve against their own step context. Callbacks should be
deterministic and concurrency-safe. This uses Go's shared runtime context, not
upstream's per-tool context or sandbox session API.

Approval applies normally: a pending or denied search discovers nothing. On a
new generation, historical approvals execute against the original registry before
step binding: a resumed search reports an unbound-tool error, while an approved
historical callee can execute without seeding discovery. Run a new search to make
its definition available again. Cancellation discards the generation's discovery
state; it does not roll back an already completed search.

## Validate and limit tools

Treat model-generated input as untrusted:

- use schema constraints for shape-level validation;
- use `ValidateInput` or checks inside `Execute` for business rules;
- pass the request context to downstream calls;
- allowlist hosts, paths, accounts, and operations;
- return bounded, model-appropriate output;
- require [approval](tool-approval.md) before consequential actions.

A tool should expose one clear capability. Avoid a general-purpose shell, SQL,
or HTTP tool unless it is tightly sandboxed and policy-controlled.

## Control tools per request

Use one reusable `ToolSet`, then narrow availability for individual calls with
`WithActiveTools`. Use `WithToolChoice` only when application policy should
force, disable, or otherwise constrain model tool selection. Provider support
for tool-choice strategies can differ.

Lifecycle hooks support input streaming and observability. Keep hook handling at
an infrastructure boundary so ordinary tool business logic remains focused on
validation and execution.

## Reference

- [`TypedTool`](https://pkg.go.dev/github.com/grafana/ai-sdk#TypedTool)
- [`Tool` and `ToolSet`](https://pkg.go.dev/github.com/grafana/ai-sdk#Tool)
- [`ToolExecutionOptions`](https://pkg.go.dev/github.com/grafana/ai-sdk#ToolExecutionOptions)
- [`ToolSearch`](https://pkg.go.dev/github.com/grafana/ai-sdk#ToolSearch)
- [`ToolDescriptionFunc`](https://pkg.go.dev/github.com/grafana/ai-sdk#ToolDescriptionFunc)

---

← [Build application workflows](../README.md#build-application-workflows) · [Docs index](../README.md) · [Tool approval →](tool-approval.md)
