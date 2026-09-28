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
provider, not by your `Execute` function.

## Route tools through callers

Use caller routing when the model should call a local or provider caller instead
of seeing every application tool directly. Configure the caller tool in the
`ToolSet`, then map each callee to the callers allowed to invoke it. A callee
listed only under a local caller is omitted from the provider's direct tool
list but remains available to that caller's bound tool set. Add the direct-call
sentinel when the callee should also stay model-visible. Active-tool settings
are applied per step before binding callers.

A provider caller can prepare the routed tool's provider options, such as
provider-specific `allowedCallers`. Manually supplied provider options pass
through when no caller preparation is configured; otherwise the preparation
callback controls the resulting options. Different providers use different
caller names and semantics. Caller routing mirrors upstream exposure and
binding; it does **not** independently authorize an unexpected provider-emitted
call against caller metadata. Enforce any security policy required by your
application within the tool's executor and approval policy.

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

---

← [Build application workflows](../README.md#build-application-workflows) · [Docs index](../README.md) · [Tool approval →](tool-approval.md)
