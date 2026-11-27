# Design

## Context

The registered Bedrock provider (`@ai-sdk/amazon-bedrock` 5.0.142 at `0e8e0543`) handles both imports through `doGenerate`: guardrail intervention retains trace and usage; the JSON response tool retains the preceding text and appends the JSON as text. Both Go and TypeScript emit `amazonBedrock` and `bedrock` result metadata.

## Goals / Non-Goals

Make unary replay independently enforce its result evidence. Do not expand unary support beyond the currently supported fixtures, change either provider, or change stream replay.

## Decisions

- Route unary cases to the shared runner even if there is no snapshot. The runner reads the snapshot before invoking `DoGenerate` and reports the file and regeneration command if it cannot be read.
- Keep the existing ordered content, finish reason, usage, optional metadata and optional warnings projection. Include all namespaces on both sides. Exclude transport-only request/response debug data as before; outbound requests have a separate comparison.
- Replay temporary copies through the same Bedrock Go test entrypoint to prove that missing evidence and a wrong metadata expectation cause a failure.

## Risks / Trade-offs

The fixture inventory already detects missing files. The replay test must still fail independently, including when the inventory is not run. A matching replay does not prove live provider behavior.
