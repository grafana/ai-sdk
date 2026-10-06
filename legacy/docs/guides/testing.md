# Testing model-backed code

Use a deterministic `provider.LanguageModel` for application tests. Keep a
smaller set of credentialed integration tests for provider configuration and
deployment wiring.

## Inject the model

Pass a `provider.LanguageModel` into the service or handler being tested. A fake
model can return a fixed provider stream:

```go
type fakeModel struct{}

func (fakeModel) SpecificationVersion() string { return "v4" }
func (fakeModel) Provider() string             { return "test" }
func (fakeModel) ModelID() string              { return "fixed-response" }
func (fakeModel) SupportedURLs() map[string][]*regexp.Regexp {
	return nil
}

func (fakeModel) DoGenerate(
	context.Context,
	provider.CallOptions,
) (*provider.GenerateResult, error) {
	return nil, errors.New("unexpected DoGenerate call")
}

func (fakeModel) DoStream(
	_ context.Context,
	_ provider.CallOptions,
) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 5)
	stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
	stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "ready"}
	stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"}
	stream <- provider.StreamPart{
		Type:         provider.PartFinish,
		FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
		Usage:        &provider.Usage{},
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}
```

`GenerateText` calls the model through `DoStream`, so the fake provides its
response there.

## Assert application behavior

```go
result, err := aisdk.GenerateText(context.Background(), fakeModel{},
	aisdk.WithModelMessages(provider.UserText("status")),
)
require.NoError(t, err)
assert.Equal(t, "ready", result.Text)
```

Add fields to the fake when a test needs to capture `provider.CallOptions`,
enforce call count, return an error, or produce different streams across tool
steps. Keep each fake focused on the behavior under test.

## Test HTTP streams

Use `httptest.ResponseRecorder` around the same handler used in production.
For UI-message SSE, assert `text/event-stream`, the protocol header, relevant
chunk ordering, safe error text, and the final `[DONE]` sentinel. For the plain
text stream used by `useCompletion` and `useObject`, assert `text/plain` and the
streamed body; that format has no protocol header or `[DONE]` sentinel.

For browser-facing behavior, run the registered `@ai-sdk/react` hooks against Go
test endpoints. Use this integration path for frontend-visible framing and state
transitions; JSON assertions cover lower-level protocol details.

## Test tools separately and in orchestration

Test a tool's validation, authorization, idempotency, and executor as ordinary
Go code. Then add orchestration tests for the model/tool interaction:

- valid and invalid tool input;
- tool execution errors;
- approval and denial;
- parallel calls where supported;
- stop conditions and maximum steps;
- cancellation while a tool is running.

Never let a unit test call a destructive production tool target.

## Separate test layers

- **Unit tests:** application policy, tools, result handling, and deterministic
  model streams.
- **HTTP tests:** headers, SSE framing, cancellation, and client-safe errors.
- **Integration tests:** real provider credentials or frontend hooks, run in a
  controlled environment.
- **Provider conformance:** request and response parity for provider authors;
  see the [conformance guide](../../test/conformance/README.md).

Avoid snapshotting entire responses when a smaller semantic assertion will be
more stable. Use conformance fixtures when exact wire bytes are the contract.

---

← [Middleware](../middleware/overview.md) · [Docs index](../README.md) · [Structured logging →](../middleware/structured-logging.md)
