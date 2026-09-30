## MODIFIED Requirements

### Requirement: Unary function history and selected results
Supported assistant function calls and tool-role text/json/error/text-file results SHALL retain current strict unions, required empties and file-selection/scope semantics. On configured direct Anthropic routes, assistant function-call part caller options SHALL reach native conversion with the registered consumed direct/code_execution variants and required toolId; no exact-key/direct-only competing validator SHALL be introduced. Supplied history attribution SHALL NOT enable providerExecuted, MCP, role/union/credential/unsupported result options or effectful fallback.

#### Scenario: Supplied caller history reaches native tool use
- **WHEN** either client sends built/supplied ordinary assistant function history with supported caller
- **THEN** the native Anthropic request SHALL preserve tool_use.caller and tool_id semantics without a prior response, server tool or future helper
- **AND** unknown/missing/empty forms SHALL follow verified registered consumed/ignored behavior

#### Scenario: Unsupported execution remains refused
- **WHEN** caller data accompanies a forbidden providerExecuted/MCP/role/union/effectful fallback request
- **THEN** existing guards SHALL refuse it before prohibited provider execution

### Requirement: Bounded private unary tool calls
Supported unary function calls SHALL retain private bounded DTOs/call ID/name/string input and current client execution ownership. Enabled providerExecuted/dynamic/preliminary or unsupported results SHALL fail safely before HTTP 200 rather than be stripped into a client call. Current metadata transport SHALL remain unchanged until #280; future preservation/continuation SHALL not be a foundation gate. Registered actual response identity follows the caller-response policy, excluding native transport/configuration.

#### Scenario: Enabled marker cannot become client execution
- **WHEN** output carries an enabled unsupported ownership marker
- **THEN** the whole response SHALL fail safely without a partial call or local execution

### Requirement: Unary client and provider acceptance evidence
Both registered clients SHALL prove current supported function behavior and the independent supplied caller-history/native request prerequisite through the authenticated handler/command. Native schemas/examples/strict/choice/ownership/credential/telemetry/bounds evidence SHALL remain. #303 supplied history SHALL not be presented as first-response metadata transport/continuation; #280 owns actual output-derived native roundtrip. Apache modules and fixture provenance remain independent.

#### Scenario: Caller request prerequisite is accepted
- **WHEN** supplied history/native-request tests pass before #280
- **THEN** request policy acceptance SHALL be recorded independently and output-derived continuation SHALL remain a successor handoff
