## Why

OpenAI Responses consumers cannot set code interpreter auto-container resource limits or target a Secure MCP tunnel. Invalid declarations otherwise reach the provider or silently lose their options.

## What Changes

- Project and validate code interpreter memory and network policy.
- Accept Secure MCP tunnel IDs, preserving the supported legacy target combination.
- Keep the new hosted-tool credentials out of SDK diagnostic captures.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `openai-responses-provider`: Hosted-tool declarations and diagnostics.

## Impact

Native OpenAI Responses and Mantle's shared adapter. Synthetic request tests establish adapter behavior, not live endpoint acceptance. No new public types, dependencies, history conversion, or Azure-specific tools.
