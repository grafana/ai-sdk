## ADDED Requirements

### Requirement: Native structured-output schema sanitization

On the native structured-output path, `applyResponseFormat` SHALL write a sanitized copy of `ResponseFormat.Schema` to `OutputConfig.Format`, MUST NOT mutate the caller's original schema, and SHALL NOT apply sanitization on the tool-based JSON fallback path.

The sanitizer SHALL strip the following JSON Schema validation keywords from
every schema node and append them as a human-readable summary to the node's
`description`:

- `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `multipleOf`
- `minLength`, `maxLength`, `pattern`
- `minItems`, `maxItems`, `uniqueItems`
- `minProperties`, `maxProperties`
- `not`

Boolean constraint values equal to `false` SHALL NOT be reported in the
appendix. Constraint names SHALL be rendered as space-separated lowercase
words (e.g., `minLength` -> `min length`, `exclusiveMinimum` -> `exclusive
minimum`). String constraint values SHALL be rendered verbatim; all other
values SHALL be rendered as their JSON encoding. Appendix entries SHALL be
joined with `"; "` and terminated with `"."`. When a node already has a
`description`, the appendix SHALL be appended after a newline.

The sanitizer SHALL preserve `$schema`, `$id`, `title`, `description`,
`default`, `const`, `enum`, `type`, and `required` keywords.

The sanitizer SHALL recurse into composition (`anyOf`, `oneOf`, `allOf`),
`items`, `properties`, `definitions`, and `$defs`. `oneOf` SHALL be rewritten
as `anyOf` on the output. A node containing `$ref` SHALL short-circuit and
emit only `{ "$ref": <value> }`, dropping all sibling keywords. Object nodes
(those with `type: "object"` or a non-nil `properties`) SHALL have
`additionalProperties: false` set on the output regardless of the input.

The sanitizer SHALL retain `format` values from the supported set
(`date-time`, `time`, `date`, `duration`, `email`, `hostname`, `uri`, `ipv4`,
`ipv6`, `uuid`) and SHALL drop other `format` values, appending them to
`description` as `format: <value>`.

#### Scenario: Numeric constraints stripped and summarized

- **WHEN** `applyResponseFormat` runs on a native-capable model with a schema
  whose `properties.recurringIntervalMinutes` is `{type: "number", minimum: 1,
  maximum: 60, exclusiveMinimum: 0, exclusiveMaximum: 120}`
- **THEN** the schema written to `OutputConfig.Format` SHALL omit `minimum`,
  `maximum`, `exclusiveMinimum`, `exclusiveMaximum` on
  `recurringIntervalMinutes`
- **AND** `recurringIntervalMinutes.description` SHALL equal
  `"minimum: 1; maximum: 60; exclusive minimum: 0; exclusive maximum: 120."`
- **AND** the original schema passed in by the caller SHALL be unchanged

#### Scenario: String constraints and unsupported format moved to description

- **WHEN** the schema declares `{type: "string", description: "A URL slug",
  minLength: 1, maxLength: 20, pattern: "^[a-z0-9-]+$", format: "regex"}`
- **THEN** the sanitized node SHALL omit `minLength`, `maxLength`, `pattern`,
  and `format`
- **AND** `description` SHALL equal `"A URL slug\nmin length: 1; max length: 20;
  pattern: ^[a-z0-9-]+$; format: regex."`

#### Scenario: oneOf rewritten as anyOf

- **WHEN** the schema is `{oneOf: [{type: "string", minLength: 1}, {type:
  "number", minimum: 0}]}`
- **THEN** the sanitized schema SHALL contain `anyOf` (not `oneOf`) with each
  branch sanitized

#### Scenario: $ref short-circuits

- **WHEN** a node is `{$ref: "#/$defs/Foo", minLength: 1}`
- **THEN** the sanitized node SHALL be `{$ref: "#/$defs/Foo"}` with all
  sibling keywords dropped

#### Scenario: Object nodes get additionalProperties: false

- **WHEN** the sanitizer visits a node whose `type` is `"object"` or that has
  a non-nil `properties`
- **THEN** the sanitized node SHALL set `additionalProperties` to `false`,
  including when the input did not specify it

#### Scenario: Recursion into definitions, $defs, items, and composition

- **WHEN** the schema contains `$defs.PositiveInteger = {type: "integer",
  minimum: 1}` and a property `tags = {type: "array", minItems: 2, maxItems:
  4, uniqueItems: true, items: {anyOf: [{type: "string", minLength: 1},
  {type: "number", maximum: 10}]}}`
- **THEN** the sanitized schema SHALL recursively strip and summarize
  constraints inside `$defs`, `items`, and each `anyOf` branch using the same
  rules

#### Scenario: Tool-fallback path is not sanitized

- **WHEN** `applyResponseFormat` falls back to injecting the `"json"` tool
  because the model does not support native structured output
- **THEN** the schema set on the synthetic tool's `InputSchema` SHALL be the
  unsanitized schema (matching upstream's behavior)

#### Scenario: Supported format values preserved

- **WHEN** a node declares `{type: "string", format: "email"}`
- **THEN** the sanitized node SHALL retain `format: "email"` and SHALL NOT
  emit a `format: ...` entry in `description`

#### Scenario: Sanitizer is non-mutating

- **WHEN** `applyResponseFormat` runs sanitization on a caller-provided schema
- **THEN** the caller's schema (e.g., as later used by orchestration-layer
  result validation) SHALL be byte-identical to what it was before the call
