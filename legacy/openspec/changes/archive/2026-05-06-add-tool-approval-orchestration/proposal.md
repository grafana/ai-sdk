## Why

The Go SDK has protocol pieces for human-in-the-loop tool approval, but `StreamText` cannot yet request approval, defer tool execution, or resume approved and denied tool calls on the next request. The upstream Vercel AI SDK has since evolved beyond the old issue description, so this change should align with the current local upstream beta rather than the stale `tool-approval-result` model.

## What Changes

- Add tool approval configuration for local tools so callers can require user approval per tool call.
- Teach orchestration to emit approval requests instead of executing blocked local tools, while letting non-blocked tools execute normally.
- Teach orchestration to collect approval responses from prior messages before the next model call, execute approved local tools, and synthesize execution-denied results for denied tools.
- Preserve UI protocol compatibility by producing `tool-approval-request` and `tool-approval-response` UI chunks and assembling approval state on tool invocation parts.
- Align provider/content protocol with upstream by modeling `tool-approval-request` as request state and `tool-approval-response` as the response prompt part; remove the obsolete `tool-approval-result` stream part from the provider API and wire tests.
- Keep provider-executed approval response handling limited to provider prompt conversion; local approval responses should not be forwarded to providers after they have been resolved into tool results.

## Capabilities

### New Capabilities
- `tool-approval-orchestration`: Human-in-the-loop approval orchestration for local tool calls in streaming text generation.

### Modified Capabilities
- `provider-v4-content-model`: Align tool approval content and stream part modeling with current upstream semantics, including removing the obsolete `tool-approval-result` stream part and adding assistant-side approval request content.
- `provider-wire`: Keep provider wire round-trip coverage aligned with the revised provider stream part set and approval fields.
- `conformance-testing`: Extend recorded fixture coverage and conformance config support for tool approval request, approved execution, and denied execution scenarios.

## Impact

- Root orchestration: `Tool`, `StreamText`, step continuation, result content construction, message appending, and UI chunk translation/assembly.
- Provider types: `StreamPartType`, `ContentPartType`, constructors, JSON/wire tests, and provider request conversion warnings.
- Anthropic provider: prompt conversion must skip or degrade unsupported approval request parts consistently while preserving provider-executed approval responses.
- Tests: unit coverage for approval decision resolution, first-call request emission, second-call resumption, denied synthetic results, UI chunks, wire round-trips, upstream-aligned removal of `tool-approval-result`, and recorded conformance fixtures comparing Go output against upstream TypeScript output for request/approved/denied approval flows.
