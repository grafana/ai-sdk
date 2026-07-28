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

Run [`examples/tools-agent`](../../examples/tools-agent) for a complete
multi-tool flow.

## Decide where execution happens

A tool with `Execute` runs in the Go process. The SDK returns its output to the
model and can continue to another step.

A tool without `Execute` is external. Its call is emitted to the result stream
and the current loop stops so a browser, queue worker, or another service can
handle it. Resume with the resulting conversation state after the external
action completes.

Some providers also supply provider-executed tools such as web search or code
execution. Those are configured through the provider package and run by the
provider, not by your `Execute` function.

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
