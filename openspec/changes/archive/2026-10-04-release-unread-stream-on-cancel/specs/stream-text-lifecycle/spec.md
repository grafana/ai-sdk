## ADDED Requirements

### Requirement: Cancellation releases an unread stream

A consumer that stops reading `FullStream` SHALL NOT keep a canceled run alive. While a part fits in the stream buffer it SHALL be delivered, including parts produced after cancellation, so a reading consumer still receives `abort` and `finish`. When the buffer is full and the run's context is canceled, whether by the caller or by a configured total, step, first-chunk or chunk timeout, the part SHALL be dropped so the run goroutine returns, `FullStream` closes, and `Wait` and the blocking accessors return.

#### Scenario: Abandoned stream finishes after cancellation

- **WHEN** a consumer reads one part from `FullStream`, stops reading until the buffer is full, and cancels the context
- **THEN** the run goroutine returns, `FullStream` is closed, and `Wait()` returns

#### Scenario: Abandoned stream finishes after a timeout

- **WHEN** a consumer reads one part from `FullStream`, stops reading until the buffer is full, and a configured total timeout then expires without the caller canceling
- **THEN** the run goroutine returns, `FullStream` is closed, and `Wait()` returns

#### Scenario: A reading consumer still receives post-cancellation parts

- **WHEN** the context is canceled while a consumer is still reading `FullStream` and the buffer has room
- **THEN** the `abort` and `finish` parts are delivered to that consumer
