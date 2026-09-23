## MODIFIED Requirements

### Requirement: Streaming function request parity
Streaming requests SHALL accept WP11's supported function definitions, choices and prompt result subset together with WP13's provider definitions and provider-result continuation, through the same complete validation and explicit mapping used for unary. Deferred families SHALL remain unsupported before resolution.

#### Scenario: Same request in both modes
- **WHEN** supported tool options are sent in unary and streaming modes
- **THEN** both SHALL map equivalent provider options and only the model invocation mode SHALL differ

### Requirement: Basic streamed calls and results
Private DTOs SHALL encode function and provider calls and correlated results with non-null JSON result and optional isError. Execution/dynamic markers SHALL be preserved on registered arms under gateway-provider-tools, including the distinction between absent and explicit false input-start dynamic. Correlation SHALL include current-stream calls and eligible unresolved provider-executed calls in request history without requiring repeated calls. Results SHALL permit preliminary replacements followed by a final result; duplicate calls, results unmatched against both sources, name mismatches and results for completed IDs SHALL fail safely. Complete client-executed and provider-executed calls SHALL not require a result before finish; provider execution may complete in a later request. Finish SHALL reject an unfinished preliminary series. History-derived state SHALL be bounded by request bytes, current-stream state by the part limit, and neither SHALL persist between requests. Arbitrary provider metadata SHALL be omitted; reviewed tool metadata SHALL follow gateway-provider-tools projection and bounds.

#### Scenario: Standalone call awaits client execution
- **WHEN** a complete function call with no incremental input events is followed by valid finish
- **THEN** the client SHALL receive the call and finish without requiring a server result

#### Scenario: Basic result transport
- **WHEN** a recording model emits a call followed by one matching basic JSON result
- **THEN** both clients SHALL receive equivalent call/result values in order

#### Scenario: Deferred provider result without repeated call
- **WHEN** a complete provider-owned call reaches finish without a result and appears unresolved in the next request history
- **THEN** its matching success or error result SHALL be accepted in the next stream without a repeated call, while results matching completed or client-owned historical calls SHALL fail safely

#### Scenario: Enabled execution marker
- **WHEN** providerExecuted or dynamic is true on a supported provider call or input-start
- **THEN** both clients SHALL receive that marker without converting provider ownership into local execution

#### Scenario: Preliminary results replace before final
- **WHEN** a call emits several preliminary results and one final result
- **THEN** event order SHALL be retained, previews SHALL not close the call and subsequent results SHALL fail safely
