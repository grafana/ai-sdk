## Why

Bedrock Converse omits the second unsupported-tool warning on Anthropic models and adds non-upstream details on other model families (#631). Warning order is observable through both generate and stream.

## What Changes

- Match the registered Bedrock 5.0.130 unsupported-provider-tool warning lists and ordering.
- Preserve request filtering and the existing unsupported web-tool diagnostics.
- Verify unary and streaming warning delivery with synthetic HTTP tests.

## Capabilities

### New Capabilities

### Modified Capabilities

- `bedrock-provider`: unsupported provider-tool warning selection and ordering.

## Impact

Only Bedrock warning construction changes. No API, dependency, fixture provenance, or request filtering changes are intended. Broader tool-choice request parity is out of scope.
