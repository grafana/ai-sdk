# Spec Delta

## MODIFIED Requirements

### Requirement: Bounded status and error classification

Status labels SHALL be `success`, `error`, or `canceled`. Error type labels SHALL be `none`, `api_call_error`, `context_canceled`, `context_deadline_exceeded`, `provider_stream_error`, or `other`. Status code SHALL be the decimal HTTP status from `*provider.APICallError`, or `none` if unavailable. Finish reason SHALL be unified `stop`, `length`, `content-filter`, `tool-calls`, `error`, `interrupted`, or `other`, or `none` if unavailable. Unknown unified values SHALL map to `other`.

#### Scenario: API call error status code is recorded

- **WHEN** a generate or stream call fails with a `*provider.APICallError` whose `StatusCode` is `429`
- **THEN** final request metrics SHALL use `error_type="api_call_error"`
- **AND** final request metrics SHALL use `status_code="429"`
- **AND** duration metrics SHALL use `status="error"` without `error_type` or `status_code` labels

#### Scenario: Context cancellation is canceled status

- **WHEN** a call fails because the context is canceled
- **THEN** final request and duration metrics SHALL use `status="canceled"`
- **AND** final request metrics SHALL use either `error_type="context_canceled"` or `error_type="context_deadline_exceeded"` according to the cancellation cause
- **AND** duration metrics SHALL NOT include an `error_type` label

#### Scenario: Error messages are not labels

- **WHEN** an error contains a URL, header value, response body, or detailed message
- **THEN** no metric label SHALL contain those values

#### Scenario: Errors map to the bounded vocabulary
- **WHEN** an error is classified for request metrics
- **THEN** `context.Canceled` SHALL map to `status="canceled"`, `error_type="context_canceled"`
- **AND** `context.DeadlineExceeded` SHALL map to `status="canceled"`, `error_type="context_deadline_exceeded"`
- **AND** errors matching `*provider.APICallError` SHALL map to `status="error"`, `error_type="api_call_error"`
- **AND** stream error parts without an API call error SHALL map to `status="error"`, `error_type="provider_stream_error"`
- **AND** all other errors SHALL map to `status="error"`, `error_type="other"`
