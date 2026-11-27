## Context

The registered `@ai-sdk/openai@4.0.122` parser reads an optional, nullable array in unary bodies and in `response.completed`, `response.incomplete` and `response.failed`. The provider exposes it under its active options namespace.

## Goals / Non-Goals

Match the registered provider's projection for valid metadata, including empty arrays and nullable IDs. Leave request shape, finish and preflight behavior, existing metadata, and UI message serialization unchanged.

## Decisions

Use the shared response metadata builder for both operations. Read the field from the OpenAI Go SDK's decoded extra fields and project the upstream entry shape, rather than changing the SSE decoder or parsing the full body again. Do not add a new public Go type for opaque provider metadata.

## Risks / Trade-offs

The SDK does not currently declare this field. Its extra fields retain raw JSON even when marked invalid because the SDK did not decode them into a known type. Read the raw value without the typed validity check and keep malformed values out of the optional metadata without changing response error handling. Synthetic unary and SSE tests guard against an SDK update changing how it exposes the field.
