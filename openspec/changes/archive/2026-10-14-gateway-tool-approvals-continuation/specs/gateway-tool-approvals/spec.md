## ADDED Requirements

### Requirement: Registered approval transport
Gateway SHALL emit provider tool-approval-request content/events with approvalId, toolCallId and optional opaque providerMetadata in unary and streaming responses. It SHALL accept tool-role tool-approval-response with required boolean approved and optional reason/options. False SHALL remain explicit; absent and empty response reasons SHALL both mean no reason.

#### Scenario: Application history boundary
- **WHEN** application approval history is converted to a provider prompt
- **THEN** application-only approval requests and ownership fields SHALL NOT be added to the V4 prompt contract

#### Scenario: Approve and deny through both clients
- **WHEN** registered Vercel and Go clients submit true or false decisions in unary and streaming calls
- **THEN** the native adapter SHALL receive the same decision, approvalId and options, including false without omission

#### Scenario: Invalid decision or role
- **WHEN** a request omits approved, supplies null/nonboolean, or places an approval response in an invalid role
- **THEN** it SHALL fail before native invocation

### Requirement: Approval correlation and execution ownership
Output approvals SHALL correlate to available provider-owned tool-call state under existing bounded lifecycle rules. Gateway SHALL NOT execute tools or require a persistent approval session. Valid standalone V4 approval response IDs SHALL reach the native adapter; native de-duplication and stored-response semantics SHALL remain native responsibilities.

#### Scenario: Malformed output correlation
- **WHEN** a provider emits an approval request referencing an unknown/completed call or conflicts with an already emitted approval
- **THEN** unary SHALL fail before commitment and streaming SHALL use its existing safe terminal error path without fallback replay

#### Scenario: Provider-native continuation
- **WHEN** a valid V4 tool approval response carries an ID without assistant approval-request history
- **THEN** it SHALL reach the native provider without a Gateway session lookup

### Requirement: Actual approved denied and mixed continuation
Both clients SHALL continue actual unary/streaming output after approved, denied and mixed decisions while retaining local/provider execution ownership, denial semantics, tool IDs, native options and opaque metadata. Direct and configured fallback SHALL accept mapped approval history and preserve existing ordering/eligibility/commitment/cancellation behavior.

#### Scenario: Native next request
- **WHEN** each client consumes approval output, appends decisions and resumes generation
- **THEN** native fake HTTP assertions SHALL prove correct next-request approval IDs, decisions, result/options and metadata, with local tools executed only by the consumer and hosted tools only by the provider

#### Scenario: Fallback and capture independence
- **WHEN** a configured first candidate fails before commitment and a later candidate emits approval output with operator capture toggled and independent consumer capture configured
- **THEN** selection SHALL preserve matching namespaces and subsequent approvals, response semantics SHALL remain identical, and no selected output/effect SHALL be replayed
