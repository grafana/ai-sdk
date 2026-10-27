## ADDED Requirements

### Requirement: Invalid media fails atomically
Generated-file validation SHALL reject missing, conflicting or unsupported data arms and malformed consumed fields. Media SHALL NOT be bounded by a unary-response or SSE-frame budget; base64 and JSON escaping expansion SHALL NOT cause a valid file to fail. Rejection SHALL use existing bounded error behavior and SHALL NOT emit a partial file or successful finish.

#### Scenario: Large unary file
- **WHEN** valid generated media and its metadata encode above the former unary response budget
- **THEN** the complete response SHALL be written with HTTP 200 and the unchanged media

#### Scenario: Large stream file
- **WHEN** a selected valid generated file encodes to a frame above the former 1 MiB limit
- **THEN** the complete file event SHALL be written and the stream SHALL continue

#### Scenario: Invalid stream file
- **WHEN** a selected generated file has invalid media fields
- **THEN** the service SHALL emit only the existing bounded terminal error behavior, cancel the producer and close owned resources
- **AND** it SHALL NOT replay the selected generation through another candidate

#### Scenario: Unsupported output arm
- **WHEN** a generated file supplies reference, text, missing data or conflicting sources
- **THEN** the service SHALL fail through its adapter error path without projecting an invented data/URL value

## REMOVED Requirements

### Requirement: Invalid or oversized media fails atomically
**Reason**: Generated media is no longer bounded by the unary response or SSE frame budget.
**Migration**: Replaced by "Invalid media fails atomically". Size is bounded by gateway-resource-policy.
