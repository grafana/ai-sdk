## ADDED Requirements

### Requirement: Tool choice none keeps tools

When `ToolChoice` is `none` and the request has tools, `buildParams` SHALL keep the tools and set `tool_choice` to `{"type":"none"}` on the direct and Vertex transports, so tool definitions stay in the prompt cache prefix. `DisableParallelToolUse` SHALL NOT replace it. Without tools, `tool_choice` SHALL be omitted. Upstream `@ai-sdk/anthropic` removes the tools; `test/conformance/upstream.yaml` records the difference.

#### Scenario: None with tools

- **WHEN** a request has a function tool and `ToolChoice` is `none`
- **THEN** the tools SHALL be sent and `tool_choice` SHALL be `{"type":"none"}`

#### Scenario: None without tools

- **WHEN** a request has no tools and `ToolChoice` is `none`
- **THEN** `tool_choice` SHALL be omitted
