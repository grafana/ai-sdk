# stream-worker-panic-containment Specification

## Purpose

Keep a panic while producing, forwarding or observing one model stream on the Gateway path from terminating the process or affecting other requests.

## Requirements

### Requirement: Stream workers contain panics

The stream worker goroutines that produce or forward provider stream parts on the Gateway path SHALL recover a panic, emit at most one terminal error part when the consumer can still receive, close their output and release upstream resources through their existing ownership paths. The terminal part SHALL carry a non-retryable `APICallError` without a status code whose cause is the internal `ErrPanic` sentinel, and no part SHALL follow it. A panic while closing a provider response body SHALL be discarded, whether the worker or a cancellation callback closes it.

#### Scenario: Transport reader panics mid-stream
- **WHEN** reading a provider response body panics after `DoStream` returned a stream
- **THEN** the stream SHALL end with one terminal error part matching `ErrPanic`, the response body SHALL be closed once, and the process SHALL keep running

#### Scenario: Stream adapter panics
- **WHEN** converting a provider event into stream parts panics
- **THEN** the stream SHALL end with one terminal error part matching `ErrPanic` and its output SHALL close
- **AND** a transport reader still waiting on the response body SHALL be released

#### Scenario: Response body close panics
- **WHEN** closing the provider response body panics, after the stream finished, after a reader panic or on cancellation
- **THEN** the process SHALL keep running and the stream SHALL end as it would have without the close panic

### Requirement: Contained panics fail setup with the sentinel and discard the value

Anthropic and OpenAI Responses read the response before `DoStream` returns, and a worker panic before it returns SHALL fail stream setup with the plain `ErrPanic` sentinel, even when frames have already arrived. OpenAI-compatible returns its stream before reading the body, so the same panic SHALL instead end that stream with the terminal error part. The recovered value SHALL NOT be retained, formatted or logged.

#### Scenario: Panic before the stream is returned
- **WHEN** the Anthropic or OpenAI Responses transport reader panics before `DoStream` returns, including after OpenAI's `response.created` while preflight is still reading
- **THEN** `DoStream` SHALL return the `ErrPanic` sentinel without a stream, and no error SHALL contain the recovered value

#### Scenario: Panic before the first frame in an eager stream
- **WHEN** the OpenAI-compatible stream worker panics before any frame arrives
- **THEN** `DoStream` SHALL succeed and its stream SHALL contain a stream start followed by the terminal error part matching `ErrPanic`

### Requirement: Gateway reports a contained panic as an internal error

When a model stream ends with a contained-panic error part, the Gateway SHALL end that response with one internal error frame. Other requests on the same handler SHALL be unaffected.

#### Scenario: Concurrent requests
- **WHEN** one streaming request's provider worker panics while another request is in flight on the same handler
- **THEN** the first response SHALL end with exactly one internal error frame that does not contain the recovered value
- **AND** the second response SHALL complete with a finish

### Requirement: Observation panics do not fail a stream

The logger, Prometheus and Agent Observability stream tees SHALL contain a panic in their observation of a stream and keep forwarding its parts unchanged. They SHALL NOT report a contained provider panic as a provider API error: the logger SHALL report it as `internal_error` and Prometheus as `other`. Checking whether an error is a contained panic SHALL NOT panic, in the logger, Prometheus or the Gateway, even when a method in the error chain panics.

#### Scenario: Malformed error chain
- **WHEN** a stream error part's cause is a typed nil `*APICallError`
- **THEN** the logger SHALL forward the part and log the stream's end as an `api_call_error`, Prometheus SHALL forward the part and count it as `api_call_error`, and the Gateway SHALL send one safe error frame and continue the stream

#### Scenario: Observation callback panics
- **WHEN** a logger clock, Prometheus normalizer or Agent Observability tracer panics while a stream is observed
- **THEN** every stream part SHALL still be forwarded and the stream SHALL close
