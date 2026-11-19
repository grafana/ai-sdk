## Why

When Grafana answers before the Gateway, for example after a wrong base URL or a namespace the token does not belong to, the Go Grafana client reports `grafana: invalid Gateway error fields` with an empty ResponseBody (#873). The message blames the Gateway and drops Grafana's own explanation, so a common setup mistake is hard to diagnose.

## What Changes

- A JSON non-2xx body with no `error` member is treated as a Grafana response. The client returns a non-retryable `*provider.APICallError` with the HTTP status, a local message that says Grafana answered, Grafana's top-level `message` when it is valid public text, and the complete bounded body in ResponseBody.
- Bodies that carry an `error` member keep today's Gateway envelope classification, including `{"error":null}` and invalid registered fields.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `grafana-gateway-client`: Report Grafana-produced error responses as Grafana errors with their message and body.

## Impact

Only `providers/grafana` changes. No public API, category or retryability changes: Grafana responses were non-retryable protocol errors before and stay so, including Grafana's 429 for an oversized request body.

Upstream `@ai-sdk/gateway` 4.0.146 (`createGatewayErrorFromResponse`) turns a body that fails its error schema into a `GatewayResponseError` with a local message and the raw response retained. Retaining the body and status matches that. Appending Grafana's `message` to the primary text is an intentional deviation: upstream has no Grafana in front of the Gateway, and Grafana's `message` is the field Grafana writes for people to read.
