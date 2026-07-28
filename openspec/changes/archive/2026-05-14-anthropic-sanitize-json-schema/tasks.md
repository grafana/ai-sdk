## 1. Sanitizer implementation

- [x] 1.1 Create `providers/anthropic/sanitize_json_schema.go` with package-private constants for `descriptionConstraintKeys` (ordered) and `supportedStringFormats` (set).
- [x] 1.2 Implement `sanitizeJSONSchema(map[string]any) map[string]any` returning a freshly constructed map; preserve `$schema`, `$id`, `title`, `description`, `default`, `const`, `enum`, `type`, `required`.
- [x] 1.3 Implement `$ref` short-circuit (emit only `{"$ref": value}`).
- [x] 1.4 Implement composition handling: copy `allOf` element-wise; rewrite `oneOf` to `anyOf`; pass `anyOf` through element-wise; recurse `definitions` and `$defs` value-wise.
- [x] 1.5 Implement object handling: when `type == "object"` or `properties` is set, sanitize `properties` value-wise, set `additionalProperties: false`, copy `required`.
- [x] 1.6 Implement `items` handling: support both single-schema (object) and tuple (slice) forms.
- [x] 1.7 Implement `format` handling: preserve when value is in `supportedStringFormats`; otherwise drop and append `format: <value>` to the description appendix.
- [x] 1.8 Implement `getConstraintDescription`: iterate `descriptionConstraintKeys` in declared order, skip nil/missing, skip boolean `false`, format keys with camelCase-to-space-lowercase, format values via `formatConstraintValue` (strings verbatim, others JSON-encoded). Join with `"; "`, terminate with `"."`. Return empty string if no entries.
- [x] 1.9 Implement description merging: when an appendix is produced, set the output's `description` to either the appendix alone (no prior description) or `existing + "\n" + appendix`.
- [x] 1.10 Implement helper `sanitizeDefinition(any) any` that passes through non-object values (booleans, scalars) and recurses for `map[string]any` values, mirroring upstream's tolerance for boolean schemas.

## 2. Integration

- [x] 2.1 In `providers/anthropic/convert_request.go`, call `sanitizeJSONSchema` on `schemaMap` immediately before `p.OutputConfig.Format = anthropic.BetaJSONSchemaOutputFormat(...)` (native path only). Do not sanitize the tool-fallback `InputSchema`.
- [x] 2.2 Confirm via inspection that the caller's `rf.Schema` (`json.RawMessage`) is not modified and that the orchestration-layer validators continue to see the original schema.

## 3. Tests

- [x] 3.1 Add `providers/anthropic/sanitize_json_schema_test.go` with table-driven cases mirroring upstream's snapshots: numeric constraints, string constraints + unsupported format, recursive arrays/definitions/composition, `oneOf` -> `anyOf`, non-mutation.
- [x] 3.2 Add cases covering `$ref` short-circuit, supported `format` preservation, `uniqueItems: false` skip behavior, and forced `additionalProperties: false` for object nodes that lacked it in input.
- [x] 3.3 Extend `TestBuildParams_StructuredOutput` in `convert_request_test.go`:
  - Subtest `NativeMode_SanitizesSchema` verifies that the schema set on `OutputConfig.Format` has constraint keys stripped and `description` enriched, while the caller-provided `rf.Schema` raw bytes are unchanged.
  - Subtest `ToolFallback_DoesNotSanitize` verifies that the injected `"json"` tool's `InputSchema` retains the original keywords.

## 4. Verification

- [x] 4.1 Run `make fmt` and `make vet` at the workspace root.
- [x] 4.2 Run `go test ./...` in `providers/anthropic/`.
- [x] 4.3 Run `make test` at the workspace root to confirm root-module tests still pass.
