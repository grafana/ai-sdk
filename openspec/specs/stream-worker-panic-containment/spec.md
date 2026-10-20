# stream-worker-panic-containment Specification

## Purpose

Keep a panic while producing, forwarding or observing one model stream on the Gateway path from terminating the process or affecting other requests.

## Requirements

### Requirement: Stream workers contain panics

The stream worker goroutines that produce or forward provider stream parts on the Gateway path SHALL recover a panic, emit at most one terminal error part when the consumer can still receive, close their output and release upstream resources through their existing ownership paths. The terminal part SHALL carry a non-retryable `APICallError` without a status code whose cause is the internal `ErrPanic` sentinel.

#### Scenario: Transport reader panics mid-stream
- **WHEN** reading a provider response body panics after the first event
- **THEN** the stream SHALL end with one terminal error part matching `ErrPanic`, the response body SHALL be closed once, and the process SHALL keep running

#### Scenario: Stream adapter panics
- **WHEN** converting a provider event into stream parts panics
- **THEN** the stream SHALL end with one terminal error part matching `ErrPanic` and its output SHALL close

### Requirement: Contained panics fail setup with the sentinel and discard the value

A worker panic before the first event SHALL fail stream setup with the plain `ErrPanic` sentinel. The recovered value SHALL NOT be retained, formatted or logged.

#### Scenario: Panic before the first event
- **WHEN** the transport reader panics before any event arrives
- **THEN** `DoStream` SHALL return the `ErrPanic` sentinel without a stream, and no error SHALL contain the recovered value

### Requirement: Gateway reports a contained panic as an internal error

When a model stream ends with a contained-panic error part, the Gateway SHALL end that response with one internal error frame and cancel the model call. Other requests on the same handler SHALL be unaffected.

#### Scenario: Concurrent requests
- **WHEN** one streaming request's provider worker panics while another request is in flight on the same handler
- **THEN** the first response SHALL end with exactly one internal error frame that does not contain the recovered value
- **AND** the second response SHALL complete with a finish

### Requirement: Observation panics do not fail a stream

The logger, Prometheus and Agent Observability stream tees SHALL contain a panic in their observation of a stream and keep forwarding its parts unchanged. They SHALL NOT report a contained provider panic as a provider API error: the logger SHALL report it as `internal_error` and Prometheus as `other`.

#### Scenario: Observation callback panics
- **WHEN** a logger clock, Prometheus normalizer or Agent Observability tracer panics while a stream is observed
- **THEN** every stream part SHALL still be forwarded and the stream SHALL close
