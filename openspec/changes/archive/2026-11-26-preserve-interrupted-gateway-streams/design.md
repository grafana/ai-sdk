## Reference

At the registered upstream commit [`0e8e0543`](https://github.com/vercel/ai/tree/0e8e0543aeda7d4dd4ce6eade0d5b7d5a43644c1), `@ai-sdk/anthropic@4.0.105` [appends the finish in flush](https://github.com/vercel/ai/blob/0e8e0543aeda7d4dd4ce6eade0d5b7d5a43644c1/packages/anthropic/src/anthropic-language-model.ts#L3613-L3671) without closing blocks. Its [tests](https://github.com/vercel/ai/blob/0e8e0543aeda7d4dd4ce6eade0d5b7d5a43644c1/packages/anthropic/src/anthropic-language-model.test.ts#L8226-L8372) cover output interrupted during text, reasoning and tool input. `@ai-sdk/gateway@4.0.146` [forwards those SSE parts](https://github.com/vercel/ai/blob/0e8e0543aeda7d4dd4ce6eade0d5b7d5a43644c1/packages/gateway/src/gateway-language-model.ts#L116-L185) without balancing blocks. The Go Anthropic provider already emits this shape.

## Decision

Keep this as a provider-output lifecycle decision, shared with extension observers. A valid interrupted finish discards the active text and reasoning state and marks each open input abandoned. An abandoned input remains reserved for the stream, but cannot be resumed or become a tool call; completed calls remain unchanged. The next response-metadata starts a new segment as usual. Preview checks, the other finish preconditions and after-finish rules remain unchanged.

Do not manufacture block ends. That would claim blocks completed, potentially misrepresent incomplete reasoning or tool input, and add events the upstream client does not need. ProviderWire opts into the shared interruption rule. The runtime and extension pipeline use the same per-call rule, with separate validator state. The default remains strict so native adapters and extension observers retain their existing invalid-output classification without inspecting a rejected finish. In particular, native Anthropic Messages cannot turn an interrupted segment with open blocks into a successful message when a later segment finishes normally.

## Evidence

Use synthetic provider parts with the production ProviderWire handler, not invented recorded provider fixtures. Assert frame schema, independent decoding by both Gateway clients, and unchanged observer classification and usage accounting for a valid finish.
