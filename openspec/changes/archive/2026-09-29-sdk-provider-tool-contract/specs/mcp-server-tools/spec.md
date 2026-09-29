## MODIFIED Requirements

### Requirement: MCP server configuration via provider options
The Anthropic provider SHALL support configuring MCP servers via the `MCPServers` field in `AnthropicOptions`. Each entry SHALL have `Name` and `URL` strings, optional presence-aware `AuthorizationToken *string`, and optional `ToolConfiguration` with presence-aware `Enabled *bool` and `AllowedTools` string slice. Absent token SHALL differ from explicitly empty token; absent enabled SHALL differ from explicit false. A non-nil empty `AllowedTools` slice SHALL remain an explicitly empty array. The configuration SHALL map to `BetaMessageNewParams.MCPServers` in `applyProviderOptions()` without emitting absent members. This API change replaces the previous string/bool fields; callers constructing the Go structs SHALL use pointers for explicit values. The existing beta selection behavior SHALL remain unchanged.

#### Scenario: Single MCP server with all fields
- **WHEN** `AnthropicOptions` contains one `MCPServer` with `Name: "my-server"`, `URL: "https://mcp.example.com"`, `AuthorizationToken` pointing to `"token123"`, and `ToolConfiguration` with `Enabled` pointing to true and `AllowedTools: ["tool_a", "tool_b"]`
- **THEN** `applyProviderOptions()` sets `p.MCPServers` to a single `BetaRequestMCPServerURLDefinitionParam` with the same name, URL, token, enabled flag and allowed tools

#### Scenario: Multiple MCP servers
- **WHEN** `AnthropicOptions` contains two MCP server entries
- **THEN** `applyProviderOptions()` sets `p.MCPServers` to two entries in order

#### Scenario: MCP server with minimal fields
- **WHEN** `AnthropicOptions` contains one `MCPServer` with only `Name` and `URL`
- **THEN** the optional authorization token and tool configuration SHALL be omitted from the native request

#### Scenario: No MCP servers configured
- **WHEN** `AnthropicOptions` has no `MCPServers` (nil or empty)
- **THEN** `applyProviderOptions()` SHALL NOT set `p.MCPServers` or inject the MCP beta header

#### Scenario: Allowed tools without enabled
- **WHEN** a server sets `ToolConfiguration.AllowedTools` to a non-nil empty or populated slice but leaves `Enabled` nil
- **THEN** the native request SHALL preserve the array and omit `tool_configuration.enabled`, rather than silently disabling the server

#### Scenario: Explicit disabled flag
- **WHEN** `ToolConfiguration.Enabled` points to false
- **THEN** the native request SHALL contain `tool_configuration.enabled: false`, distinct from omission

#### Scenario: Explicit empty token
- **WHEN** `AuthorizationToken` points to the empty string
- **THEN** the native request SHALL retain `authorization_token: ""`, distinct from omission
