# Validate and preserve allowed-tool subsets

Official Chat clients can restrict callable tools while retaining full tool
definitions. Add typed compatible selection and reuse Responses AllowedTools.
Keep common auto/required choices and apply output membership before streaming
any tool payload. Anthropic filters only attempt definitions, preserving history
but with a documented prompt/cache difference. Raw tool_choice is not introduced.
