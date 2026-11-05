## Context

The registered baseline is @ai-sdk/openai 4.0.96, commit b7f6255acaae7724adc0fd37aac3dae3f3466578. Its convert-to-openai-responses-input.ts and matching tests serialize both error variants through JSON.stringify({error: output.value}), independently of output schemas. The existing Go helper is shared by function, custom and grouped parallel results. PARITY.md classifies provider adapters as mixed provider-implementation coverage.

## Goals / Non-Goals

**Goals:** preserve failure values and all surrounding scalar conversion semantics in unary and streaming requests, under active OpenAI/Azure options.

**Non-Goals:** public API changes, new multipart behavior, live-provider acceptance claims or invented conformance recordings.

## Decisions

Separate the error cases in toolResultOutputString and encode a single error property. Keep JSON values as json.RawMessage to preserve numeric precision and structured values; absent or empty raw JSON represents null in the error envelope. Keep successful JSON conversion unchanged. Centralizing conversion avoids divergent custom and parallel encoders. Output-schema quoting applies only to text and denial; errors are already JSON objects.

Use focused synthetic HTTP transport tests first because the issue allows them and no provenance-valid provider recording establishes this boundary. Assert exact emitted input items and stream flag, cache namespace selection/precedence, unchanged caller inputs, schema behavior and grouping. Retain existing denial/multipart tests and run the module suite. Run parity-check and report any environmental blocker rather than inventing fixtures.

## Risks / Trade-offs

- Nested error properties intentionally remain nested, matching upstream.
- Synthetic tests establish emitted request shape, not live-provider acceptance; authentic recordings are outside this change.
