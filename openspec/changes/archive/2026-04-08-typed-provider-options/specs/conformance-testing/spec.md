## MODIFIED Requirements

### Requirement: Test case configuration
Each test case directory SHALL contain a `config.yaml` file that declares the replay configuration. The YAML SHALL specify: `model` (string), and optionally: `prompt` (string, for recording and documentation), `stopWhenStepCount` (integer, default 1), `providerOptions` (nested map), and `tools` (map of tool name to tool definition with `description`, `inputSchema` JSON schema, and `mockResults` list). The provider SHALL be inferred from the parent directory structure, not from the YAML. When `providerOptions` is specified in YAML, the Go test SHALL marshal each provider namespace value to JSON and wrap it as `provider.RawProviderOption` for use with the typed `map[string]provider.ProviderOption` field on `StreamTextParams`.

#### Scenario: Minimal config
- **WHEN** a config specifies only `model`
- **THEN** it is a valid single-step test case with no tools and no special provider options

#### Scenario: Tool call config
- **WHEN** a config specifies `tools` with `mockResults` and `stopWhenStepCount` > 1
- **THEN** the Go test SHALL register tools with `Execute` functions that return mock results from the list in order, one per tool execution

#### Scenario: Provider options config
- **WHEN** a config specifies `providerOptions` with nested YAML map values
- **THEN** the Go test SHALL marshal each namespace value to JSON, wrap as `RawProviderOption`, and pass the resulting `map[string]provider.ProviderOption` to `StreamTextParams.ProviderOptions`

#### Scenario: Provider inference
- **WHEN** a test case is located at `anthropic/upstream/tool-call/`
- **THEN** the Go test SHALL use the Anthropic provider without needing a `provider` field in `config.yaml`
