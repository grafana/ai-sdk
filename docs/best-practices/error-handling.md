# Error handling

Errors appear differently before a response starts, during an asynchronous
stream, and after a blocking generation. Handle each boundary deliberately.

## Blocking calls return errors

`GenerateText` and `output.GenerateObject` return generation failures directly:

```go
result, err := aisdk.GenerateText(ctx, model, opts...)
if err != nil {
	return fmt.Errorf("generating summary: %w", err)
}
```

Schema construction and provider constructors that perform setup can also return
errors before any model call begins.

## Streaming calls report errors through the result

`StreamText` returns immediately. Observe errors while consuming the stream and
check the final result:

```go
result := aisdk.StreamText(ctx, model,
	aisdk.OnError(func(err error) {
		log.Printf("model stream: %v", err)
	}),
)

for part := range result.FullStream() {
	if streamErr, ok := part.(aisdk.StreamError); ok {
		log.Printf("stream event: %v", streamErr.Error)
	}
}

if err := result.Err(); err != nil {
	return err
}
```

The callback is useful for telemetry; it does not consume or recover the error.
Avoid logging the same failure at every layer unless each record has a distinct
purpose.

## Map HTTP errors before and after streaming starts

Before headers are committed, validate requests and return ordinary HTTP status
codes. After an SSE response starts, an HTTP status can no longer be replaced;
the failure must be represented in the stream.

Use `OnUIMessageStreamError` to return a safe client-facing message while
retaining the original error in server telemetry:

```go
stream := result.ToUIMessageStream(
	aisdk.OnUIMessageStreamError(func(error) string {
		return "The model request could not be completed."
	}),
)
```

Never expose provider response bodies, credentials, internal URLs, or policy
details to an untrusted client.

## Classify provider failures

Providers use `provider.APICallError` for HTTP and transport failures. It carries
status, retryability, response metadata, and the original cause where available.
Use `errors.As` to inspect typed provider failures:

```go
var apiErr *provider.APICallError
if errors.As(err, &apiErr) {
	recordProviderFailure(apiErr.StatusCode, apiErr.IsRetryable)
}
```

The SDK may wrap repeated failures in `RetryError`, so preserve error chains with
`%w`. Let retry and fallback policy use the provider classification and retry
only eligible errors.

## Treat cancellation separately

```go
if errors.Is(err, context.Canceled) {
	return nil
}
if errors.Is(err, context.DeadlineExceeded) {
	recordTimeout()
	return err
}
```

Client disconnects are expected for streaming endpoints. A deadline is also an
expected control path, though frequent timeouts may indicate a service problem.
Distinguish caller cancellation, total timeout, step timeout, and idle stream
timeout in observability.

## Handle partial work

A failed multi-step run may already have emitted text or completed tools. Do not
assume an error means nothing happened. Side-effecting tools need idempotency and
a durable operation record when retries can occur outside the SDK.

---

← [Production checklist](production.md) · [Docs index](../README.md) · [Security →](security.md)
