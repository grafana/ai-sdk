## Context

At registered Vercel AI revision `0e8e0543`, OpenAI 4.0.118 validates the code interpreter container and MCP target via their argument schemas, then maps them to the Responses declaration. Amazon Bedrock 5.0.142 uses the same Responses adapter for Mantle.

## Goals / Non-Goals

Match the pinned declaration behavior and protect the new credential-bearing fields. Do not change shell arguments, result history, or legacy MCP credential capture.

## Decisions

- Use existing camelCase `provider.Tool.Args` and the installed SDK's request types. Distinguish omitted fields from empty strings and arrays. The upstream MCP validator permits URL and connector together.
- Keep real transport data intact. For recognized credential-bearing declarations, omit raw bodies, stream chunks, and provider finish extensions that could echo arbitrary values. Retain normalized output, status, retryability, and validated retry hints; strip credential fields before sending tool arguments to the logger redactor.
- Detection checks declarations built by the adapter and the final JSON body observed by SDK request capture, including JSON request options. Arbitrary mutations by caller-owned transports after capture, caller logs, and generated content are outside this diagnostic boundary. This is not a port of the upstream telemetry API.

## Risks / Trade-offs

Target validation intentionally rejects requests that Go previously projected without a target. Provider acceptance of new options on particular Mantle models is not established by synthetic requests.
