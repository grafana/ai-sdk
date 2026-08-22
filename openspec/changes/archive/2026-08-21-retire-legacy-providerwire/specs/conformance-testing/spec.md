## MODIFIED Requirements

### Requirement: AWS event-stream replay framing

The replay server SHALL support a Bedrock framing mode that serves fixture lines as AWS Smithy event-stream binary frames instead of SSE. Each fixture line MUST be encoded as a single frame with a `:event-type` header set to the outer JSON key of the line and a payload equal to the inner JSON object. The HTTP response Content-Type MUST be `application/vnd.amazon.eventstream`.

#### Scenario: Bedrock replay encodes binary frames

- **WHEN** the replay server is in Bedrock mode and a fixture line is `{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"hi"}}}`
- **THEN** the wire response contains a Smithy event-stream frame with `:event-type=contentBlockDelta` and JSON payload `{"contentBlockIndex":0,"delta":{"text":"hi"}}`

#### Scenario: Bedrock replay content type

- **WHEN** the replay server is in Bedrock mode
- **THEN** the HTTP response carries `Content-Type: application/vnd.amazon.eventstream`

#### Scenario: Multi-step Bedrock replay

- **WHEN** the replay server is in Bedrock mode for a multi-step case with `input-1.chunks.txt` and `input-2.chunks.txt`
- **THEN** sequential requests receive the corresponding fixture as separate event-stream binary responses

#### Scenario: Anthropic replay unaffected

- **WHEN** the replay server is in Anthropic SSE mode
- **THEN** the SSE wire format and Content-Type are unchanged from the existing behavior

### Requirement: Truncated provider stream coverage

The conformance suite SHALL include deterministic fixtures for provider streams that close without a finish part. Coverage SHALL distinguish an incomplete stream with no model output from an incomplete stream with partial model output and SHALL compare direct provider replay against the registered upstream UI chunk sequence.

#### Scenario: Empty truncated provider stream

- **WHEN** a provider response emits only administrative metadata and closes without a finish part
- **THEN** upstream expected output contains an error chunk and no finish chunk
- **AND** the Go result reports a stream error

#### Scenario: Partial truncated provider stream

- **WHEN** a provider response emits model output and closes without a finish part
- **THEN** upstream expected output retains the partial chunks, emits `finish-step`, and finishes with reason `other`
- **AND** the Go result does not report a stream error

## REMOVED Requirements

### Requirement: Grafana provider-wire conformance boundary
**Reason**: The Grafana client, legacy handler, and their transport-boundary conformance are removed.
**Migration**: Retain direct-provider request snapshots and stream conformance; strict Gateway contract evidence will be introduced in the next work package.
