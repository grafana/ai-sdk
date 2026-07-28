# Structured output

Use structured output when application code needs validated data for extraction,
classification, routing, form completion, or a typed API response.

Structured output improves shape reliability, but it does not make model values
factually correct. Continue to validate business rules before acting on them.

## Choose an output shape

- `Object[T]` for one typed value
- `Array[T]` for repeated typed values
- `Choice` for one value from a fixed set
- `JSON` for schema-constrained JSON without a Go result type

Typed objects and arrays provide compile-time result shapes. `Choice` constrains
the result to one value from a fixed set.

## Define a schema from Go

```go
type AlertTriage struct {
	Severity  string `json:"severity" jsonschema:"enum=critical,enum=warning,enum=info"`
	RootCause string `json:"rootCause" jsonschema:"description=Likely root cause in one sentence"`
}

schemaValue, err := schema.SchemaFor[AlertTriage]()
if err != nil {
	return err
}

objectOutput, err := output.Object[AlertTriage](schemaValue)
if err != nil {
	return err
}
```

Use field descriptions to provide meaning the model cannot infer from the JSON
name. Keep schemas as small as the application contract allows. Dynamic callers
can use `SchemaFromJSON` or `SchemaFromFile`.

## Generate and validate a value

```go
result, err := output.GenerateObject[AlertTriage](ctx, model, objectOutput,
	aisdk.WithSystem("Triage the alert into the required structure."),
	aisdk.WithModelMessages(provider.UserText(alertText)),
)
if err != nil {
	return err
}

triage, err := result.Object()
if err != nil {
	return err
}
```

`Object()` returns the typed value only after JSON parsing and schema validation.
Handle both the generation error and the output-access error.

Run [`examples/structured-output`](../../examples/structured-output) for a
complete extraction workflow.

## Stream structured output

Use `StreamObject` when the caller benefits from incremental progress. The final
`Object()` call blocks until a complete validated value is available:

```go
result := output.StreamObject[AlertTriage](ctx, model, objectOutput, opts...)

drained := make(chan struct{})
go func() {
	defer close(drained)
	for range result.FullStream() {
	}
}()

for partial := range result.PartialOutputStream() {
	publishProgress(partial)
}
<-drained

if err := result.Err(); err != nil {
	return err
}
triage, err := result.Object()
```

The full orchestration stream and partial-output stream are independent. Drain
the full stream concurrently whenever you consume partial output directly.
Partial JSON is incomplete by definition; use it for previews, not database
writes or irreversible actions. For array output, `output.TypedElementStream`
can emit completed elements while the full result is still being generated; it
also needs a concurrent `FullStream` consumer.

## Stream JSON to `useObject`

`useObject` consumes the same plain text JSON stream written by
`WriteTextStream`:

```go
result := output.StreamObject[AlertTriage](r.Context(), model, objectOutput, opts...)
if err := aisdk.WriteTextStream(w, result.StreamTextResult); err != nil {
	return err
}
```

The frontend still supplies its schema to `useObject`; the Go output schema
constrains and validates provider output on the server.

## Provider support

Providers differ in native structured-output support. The SDK supplies the
requested response format and reports unsupported settings or provider-specific
limitations through errors or warnings. Consult the provider page when strict
schema behavior matters.

## Reference

- [`output` package](https://pkg.go.dev/github.com/grafana/ai-sdk/output)
- [`schema` package](https://pkg.go.dev/github.com/grafana/ai-sdk/schema)

---

← [Tool approval](tool-approval.md) · [Docs index](../README.md) · [Agent loops →](agent-loops.md)
