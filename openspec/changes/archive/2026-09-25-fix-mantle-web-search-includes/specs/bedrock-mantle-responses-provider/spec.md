## ADDED Requirements

### Requirement: Mantle web-search source include compatibility
Mantle Responses SHALL configure the shared OpenAI Responses model so that
`web_search_call.action.sources` is not automatically included for a supplied
web-search tool in either unary or streaming calls. This provider-owned
restriction SHALL NOT remove the web tool, change response attribution, block
other automatic include kinds, or silently remove an explicit caller `include`
entry. The Bedrock module SHALL exercise this behavior against a publicly
resolvable OpenAI module version without workspace substitutions.

#### Scenario: Unary request accepted by strict compatibility endpoint
- **WHEN** a Mantle Responses model makes a `DoGenerate` call with a web-search tool to a synthetic endpoint rejecting `web_search_call.action.sources`
- **THEN** the request retains the web-search tool, omits the automatic web-source include, and is accepted by that endpoint

#### Scenario: Streaming request accepted by strict compatibility endpoint
- **WHEN** a Mantle Responses model makes a `DoStream` call with a web-search tool to the same strict synthetic endpoint
- **THEN** the request retains the web-search tool, omits the automatic web-source include, and its stream succeeds

#### Scenario: Caller-specified source include is preserved
- **WHEN** a Mantle caller explicitly requests `web_search_call.action.sources` via the OpenAI Responses `include` option
- **THEN** the serialized request still contains that value
- **AND** this does not assert Mantle accepts that explicitly requested value

#### Scenario: Unrelated includes and attribution survive
- **WHEN** Mantle requests code-interpreter outputs, logprobs, or stateless encrypted reasoning alongside a web-search tool
- **THEN** their existing automatic include behavior remains intact, and response attribution stays `bedrock-mantle.responses`

#### Scenario: Published module consumer boundary
- **WHEN** the Bedrock module is tested with `GOWORK=off` using its declared published OpenAI module dependency
- **THEN** Mantle unary and streaming web-tool requests retain the same automatic-source opt-out behavior as workspace tests
