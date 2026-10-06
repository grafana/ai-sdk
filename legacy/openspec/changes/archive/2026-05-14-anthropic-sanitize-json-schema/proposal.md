## Why

Anthropic's structured-output decoder rejects several JSON Schema validation
keywords (e.g. `minimum`, `maxLength`, `pattern`, `not`, unsupported `format`
values) when supplied via `output_config.format.schema`. Schemas produced by
typical generators (Zod, `invopop/jsonschema`, etc.) include these keywords, so
requests fail with HTTP 400 even though the orchestration layer still needs the
full schema for result validation. Upstream `@ai-sdk/anthropic` shipped
`sanitize-json-schema.ts` (`vercel/ai` `c012d57`, carried in `2026-04-20`) to
strip the rejected keywords from the constrained-decoder payload while
preserving the constraints as human-readable text in `description`. The Go port
must follow to stay wire-compatible and to unblock structured output against
Opus 4.7 / Sonnet 4.x.

## What Changes

- Add a sanitizer in the anthropic provider that strips JSON Schema validation
  keywords Anthropic rejects (`minimum`, `maximum`, `exclusiveMinimum`,
  `exclusiveMaximum`, `multipleOf`, `minLength`, `maxLength`, `pattern`,
  `minItems`, `maxItems`, `uniqueItems`, `minProperties`, `maxProperties`,
  `not`) and unsupported `format` values, appending the dropped constraints to
  the schema's `description` in the same wording used upstream.
- Recurse into composition (`anyOf`, `oneOf`, `allOf`), `items`,
  `properties`, `definitions`, and `$defs`; convert `oneOf` to `anyOf`;
  short-circuit `$ref` nodes; force `additionalProperties: false` on object
  nodes; preserve `$schema`, `$id`, `title`, `default`, `const`, `enum`,
  `required`.
- Apply the sanitizer only on the native structured-output path in
  `applyResponseFormat` (i.e. before `OutputConfig.Format` is assigned).
  Tool-fallback tool input schemas remain untouched, matching upstream.
- Keep the unsanitized schema available to the orchestration layer's result
  validation by sanitizing a copy.

## Capabilities

### New Capabilities

### Modified Capabilities
- `anthropic-structured-output`: Add a requirement that, on the native
  structured-output path, the schema written to `OutputConfig.Format` is
  sanitized to remove keywords Anthropic rejects, with stripped numeric/string
  constraints reflected in `description`.

## Impact

- Affects `providers/anthropic/convert_request.go` (`applyResponseFormat` call
  site only) and adds a new `providers/anthropic/sanitize_json_schema.go` with
  matching tests.
- No changes to public Go API, provider interface, or SSE wire format.
- Removes a class of HTTP 400 errors on structured-output requests against
  models that support native structured output.
