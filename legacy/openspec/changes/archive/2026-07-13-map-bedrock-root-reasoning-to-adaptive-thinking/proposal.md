## Why

Root `CallOptions.Reasoning` always produced budget-token thinking for Anthropic models on Bedrock, even when the model supports adaptive thinking. This diverged from the registered `@ai-sdk/amazon-bedrock@5.0.16` behavior and omitted the requested reasoning effort.

## What Changes

- Detect Anthropic Bedrock reasoning capabilities and model maximums using the registered upstream capability set.
- Map root reasoning to adaptive thinking plus Bedrock effort for capable models.
- Preserve correctly sized budget-token thinking for older and unknown Anthropic models.
- Merge partial explicit reasoning config fields over derived root reasoning, preserve merged fields in Nova-style requests, and clear derived values when thinking is disabled.
- Add unit and provider-request conformance coverage for capability and provider-option branches.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `bedrock-provider`: Define capability-gated root reasoning conversion for Anthropic Bedrock models.

## Impact

- `providers/bedrock` model capability detection and request conversion.
- Bedrock request-conversion unit tests.
- Bedrock conformance request snapshots generated from the registered upstream baseline.
