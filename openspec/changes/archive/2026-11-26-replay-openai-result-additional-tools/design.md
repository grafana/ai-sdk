## Context

The registered target is vercel/ai `0e8e0543` (ai 7.0.173, @ai-sdk/openai 4.0.118). Its `convert-to-openai-responses-input.ts` inserts definitions after the ordinary function-output path and after grouped wrapper output, but not after early-returning native results. Its options schema uses a stripping object at the namespace level and strict objects for each tool definition and custom format. Both Go HTTP entrypoints use the same request builder, and core already forwards supplied model messages across steps.

## Decisions

- Add a typed `AdditionalTools` slice to `OpenAIPartOptions`. Its nil value omits the field; an explicit empty slice survives JSON encoding and suppresses the OpenAI fallback when Azure is active.
- Use one validator for typed and raw definitions. Keep the contents of function parameter objects opaque. Use the SDK's additional-tools input item with validated definitions rather than the top-level tool preparer, which has different rules.
- Extend the existing tool-role continuation converter to return ordered input items. Keep the result item first; grouped children contribute one following item each in wrapper index order.
- Do not infer local tool eligibility from history definitions. Only caller-supplied history is forwarded, with existing execution and registration rules unchanged.

## Risks / Trade-offs

The next proposed upstream pin changes replay to select registered definitions rather than trusting definitions from history. This change implements the registered pin, not that later behavior. Synthetic HTTP tests can prove the request contract, but are not live Responses acceptance evidence.
