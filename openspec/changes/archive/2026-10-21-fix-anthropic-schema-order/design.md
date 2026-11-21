## Context

Registered `@ai-sdk/anthropic@4.0.71`, source `eb77f09e3c06c28e860d92e0de941b143c2eecec`, retains property insertion order in `sanitize-json-schema.ts` and passes tool schemas unchanged in `anthropic-language-model.ts`. Go map serialization alphabetizes those names.

## Decisions

Keep the existing sanitizer. Serialize its result using the caller schema's member order, following nested objects and arrays and the `oneOf` to `anyOf` rewrite. Append injected keys deterministically. Supply the resulting `json.RawMessage` to the native SDK, which rejects arbitrary schema marshalers.

For JSON fallback and function tools, pass validated original JSON through the existing native SDK override. This avoids sorting and float64 rounding without applying native-output sanitization to tools.

## Risks and Evidence Limits

Focused synthetic HTTP tests inspect actual SDK serialization; they do not establish live-provider acceptance or model quality. No matching upstream response fixture proves request-key ordering, and no recorded provider input is invented or changed. Existing semantic conformance remains applicable. Bedrock is outside this change.
