## ADDED Requirements

### Requirement: Configurable bounded stream drain
Prometheus middleware options SHALL provide an optional positive stream-drain duration. When configured, cancellation cleanup SHALL drain the immediate upstream only until channel close or the absolute deadline, including for a continuously ready channel. Zero SHALL preserve the existing direct-consumer drain behavior.

#### Scenario: Configured drain expires
- **WHEN** downstream cancellation occurs and the immediate upstream never closes or remains continuously ready
- **THEN** the Prometheus-owned drain goroutine SHALL exit no later than the configured absolute deadline
- **AND** terminal metrics, in-flight decrement, and downstream channel closure SHALL each occur exactly once without waiting for the drain

## MODIFIED Requirements

### Requirement: Stream chunk and timing metrics

For streams, the middleware SHALL increment `aisdk_model_stream_chunks_total` for every upstream `provider.StreamPart` observed and forwarded. The `chunk_type` label SHALL equal a registered `provider.StreamPartType` string for known values and `other` for every unknown value. This metric SHALL be disabled when `Options.DisableStreamChunkMetrics` is true.

The middleware SHALL observe `aisdk_model_time_to_first_output_seconds` once per stream when at least one payload-bearing stream part is observed. The observation SHALL use the elapsed seconds between starting the provider call and observing the first payload-bearing part, and SHALL be emitted at stream finalization with the final stream status label. Streams that finish, error, or cancel before any payload-bearing part SHALL NOT observe TTFT.

The middleware SHALL observe `aisdk_model_inter_chunk_delay_seconds` for gaps between consecutive payload-bearing stream parts, labeled by the current payload-bearing part type. This metric SHALL be disabled when `Options.DisableStreamChunkMetrics` is true.

Payload-bearing stream part types SHALL be `text-delta`, `reasoning-delta`, `tool-input-delta`, `tool-call`, `tool-result`, `source`, `file`, `custom`, `reasoning-file`, and `tool-approval-request`. Framing, metadata, raw, finish, and error parts SHALL NOT be payload-bearing.

#### Scenario: Chunk counter records all parts

- **WHEN** a stream emits `text-delta`, `response-metadata`, `finish`, and `error` parts
- **THEN** `aisdk_model_stream_chunks_total` SHALL increment once for each known chunk type observed

#### Scenario: Unknown chunk types use one closed label

- **WHEN** a stream emits one or more unregistered `provider.StreamPartType` values
- **THEN** `aisdk_model_stream_chunks_total` SHALL bucket every such part under `chunk_type="other"`
- **AND** no unregistered value SHALL become a metric label

#### Scenario: TTFT records first payload only

- **WHEN** a stream emits metadata parts before its first `text-delta`
- **THEN** TTFT SHALL be measured to the first `text-delta`
- **AND** metadata parts before it SHALL NOT cause a TTFT observation

#### Scenario: Inter-chunk delay records payload gaps

- **WHEN** a stream emits two consecutive payload-bearing parts with non-zero elapsed time between them
- **THEN** `aisdk_model_inter_chunk_delay_seconds` SHALL observe that elapsed time labeled by the second payload-bearing part type

#### Scenario: Stream chunk metrics can be disabled

- **WHEN** instrumentation is configured with `DisableStreamChunkMetrics` true
- **THEN** stream request, duration, in-flight, token, and TTFT metrics SHALL still be recorded
- **AND** `aisdk_model_stream_chunks_total` and `aisdk_model_inter_chunk_delay_seconds` SHALL NOT be registered or observed
