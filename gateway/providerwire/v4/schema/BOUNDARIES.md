# ProviderWire V4 serialized boundaries

This ledger describes JSON after Gateway preprocessing and `JSON.stringify` for
the package set in [`PROVENANCE.md`](../PROVENANCE.md). TypeScript declarations
are inventory inputs; the checked-in HTTP exchange and schemas are normative.
Standard objects and selected arms are closed unless this file marks a value as
open.

## Serialization rules

- JavaScript `undefined` object properties are absent. Undefined array elements
  are outside the typed contract.
- Explicit `[]`, `{}`, `false`, `0`, and empty strings remain present.
- Present typed fields reject `null` unless their type is an open JSON value.
- Dates are RFC 3339 date-time strings.
- Gateway removes `abortSignal` and converts supported inline `Uint8Array` file
  data to base64 strings before serialization.
- Semantic JSON equality is required; object-member order and insignificant
  whitespace are not.

## Open value boundaries

| Boundary | JSON domain | Null | Empty object | Notes |
| --- | --- | --- | --- | --- |
| JSONValue | any JSON value | yes | yes | Tool-call input, prompt JSON/error-JSON result values, raw/error stream values, and request/response bodies. |
| NonNullJSONValue | any JSON value except null | no | yes | Generated and streamed tool-result `result`. |
| JSONObject | object with JSONValue members | no | yes | Provider-tool args, function input examples, usage raw, and each provider namespace value. |
| JSON Schema object | object | no | yes | Function input schemas and JSON response schemas. |
| Provider reference | object with string values | no | yes | The curated projection rejects a member named `type` to preserve file-data discrimination. |

Provider options and metadata are open only as keyed namespace maps. The outer
map and each namespace object may be empty, but namespace values cannot be
scalars, arrays, or null. Headers are open string-to-string maps.

## Request root

The root is closed and requires `prompt`. These fields are optional by undefined
omission: `maxOutputTokens`, `temperature`, `stopSequences`, `topP`, `topK`,
`presencePenalty`, `frequencyPenalty`, `responseFormat`, `seed`, `tools`,
`toolChoice`, `includeRawChunks`, `headers`, `reasoning`, and `providerOptions`.
Numeric fields are JSON numbers. `includeRawChunks: false`, zero numbers, and
empty collections remain distinct from absence. `abortSignal` is never valid.

`reasoning` is one of `provider-default`, `none`, `minimal`, `low`, `medium`,
`high`, or `xhigh`. Response format is exactly `{type:"text"}` or the closed JSON
arm `{type:"json", schema?, name?, description?}`.

### Prompt messages

Every message may have optional object-valued `providerOptions` and is otherwise
closed.

| Role | Required content |
| --- | --- |
| `system` | string |
| `user` | array of `text` or `file` parts |
| `assistant` | array of `text`, `file`, `custom`, `reasoning`, `reasoning-file`, `tool-call`, or `tool-result` parts |
| `tool` | array of `tool-result` or `tool-approval-response` parts |

Empty content arrays remain representable. Exact prompt part arms are:

- `text` and `reasoning`: required string `text`; optional `providerOptions`.
- `custom`: required dotted string `kind`; optional `providerOptions`.
- `file`: required selected `data` and string `mediaType`; optional string
  `filename` and `providerOptions`.
- `reasoning-file`: required generated-file `data` and string `mediaType`;
  optional `providerOptions`; no filename.
- `tool-call`: required strings `toolCallId` and `toolName`, open JSONValue
  `input`; optional boolean `providerExecuted` and `providerOptions`.
- `tool-result`: required strings `toolCallId` and `toolName`, selected `output`;
  optional `providerOptions`.
- `tool-approval-response`: required string `approvalId` and boolean `approved`;
  optional string `reason` and `providerOptions`.

File data is one exact arm: `{type:"data",data:string}`,
`{type:"url",url:string}`, `{type:"reference",reference:ProviderReference}`,
or `{type:"text",text:string}`. Reasoning and generated files permit only data
and URL arms.

Tool-result output is one exact arm:

- `text` or `error-text`: required string `value`; optional `providerOptions`.
- `json` or `error-json`: required JSONValue `value`, including null; optional
  `providerOptions`.
- `execution-denied`: optional string `reason` and `providerOptions`.
- `content`: required array of exact text, file, or custom parts. Text requires
  `text`; file requires selected file data and `mediaType` and permits
  `filename`; custom contains no standardized payload. Each nested part may
  carry `providerOptions`; the outer content arm may not.

### Tools

- Function tool: required `type`, string `name`, and JSON Schema object
  `inputSchema`; optional string `description`, array `inputExamples` containing
  `{input: JSONObject}`, boolean `strict`, and `providerOptions`.
- Provider tool: exactly `type`, dotted string `id`, string `name`, and
  JSONObject `args`.
- Tool choice: exact `auto`, `none`, or `required` arm, or `tool` with required
  string `toolName`.

## Generate result

The root is closed and requires ordered `content`, `finishReason`, `usage`, and
`warnings`. Optional fields are `providerMetadata`, `request`, and `response`.
Warnings remains required when empty.

Generated content exact arms:

- `text` and `reasoning`: required string `text`; optional `providerMetadata`.
- `custom`: required dotted string `kind`; optional `providerMetadata`.
- `file` and `reasoning-file`: required string `mediaType` and selected data/URL
  `data`; optional `providerMetadata`.
- URL source: required `sourceType:"url"`, strings `id` and `url`; optional
  string `title` and `providerMetadata`.
- Document source: required `sourceType:"document"`, strings `id`, `mediaType`,
  and `title`; optional string `filename` and `providerMetadata`.
- Tool call: required strings `toolCallId`, `toolName`, and serialized string
  `input`; optional booleans `providerExecuted` and `dynamic`, plus
  `providerMetadata`.
- Tool result: required strings `toolCallId` and `toolName` and
  NonNullJSONValue `result`; optional booleans `isError`, `preliminary`, and
  `dynamic`, plus `providerMetadata`.
- Tool approval request: required strings `approvalId` and `toolCallId`; optional
  `providerMetadata`.

Finish reason requires `unified` from `stop`, `length`, `content-filter`,
`tool-calls`, `error`, or `other`; optional string `raw` is omitted when
undefined.

Usage requires closed `inputTokens` and `outputTokens` objects. Input counters
`total`, `noCache`, `cacheRead`, and `cacheWrite` and output counters `total`,
`text`, and `reasoning` are independently optional numbers after undefined
omission. Optional `raw` is a JSONObject.

Warning arms are exactly:

- `unsupported` or `compatibility`: required string `feature`, optional string
  `details`.
- `deprecated`: required strings `setting` and `message`.
- `other`: required string `message`.

`request` is closed with optional JSONValue `body`. `response` is closed with
optional strings `id` and `modelId`, optional RFC 3339 `timestamp`, optional
string map `headers`, and optional JSONValue `body`. A Go-only response
`provider` field is invalid.

## Stream parts

Each SSE payload is one complete closed arm. Ordering is outside JSON Schema.
The discriminator set is:

`stream-start`, `response-metadata`, `text-start`, `text-delta`, `text-end`,
`reasoning-start`, `reasoning-delta`, `reasoning-end`, `tool-input-start`,
`tool-input-delta`, `tool-input-end`, `tool-approval-request`, `tool-call`,
`tool-result`, `custom`, `file`, `reasoning-file`, `source`, `finish`, `raw`, and
`error`.

- Text/reasoning starts and ends require string `id`; deltas additionally
  require string `delta`. All permit optional `providerMetadata`.
- Tool-input start requires string `id` and `toolName`; optional
  `providerMetadata`, booleans `providerExecuted` and `dynamic`, and string
  `title`. Delta requires `id` and `delta`; end requires `id`; both permit
  `providerMetadata`.
- Generated content arms use the exact generate-result definitions above.
- `stream-start` requires `warnings`, including an explicit empty array.
- `response-metadata` permits only optional `id`, RFC 3339 `timestamp`, and
  `modelId`.
- `finish` requires `usage` and `finishReason`; optional `providerMetadata`.
- `raw.rawValue` and `error.error` are JSONValue and may be null.

Streamed/generated tool results reject null. Generated file and reasoning-file
parts permit only selected data/URL file data.

## HTTP error projection

This is a local serialized projection accepted by the pinned stock client, not a
provider V4 type or private-server capture. The closed root requires `error` and
permits optional string `generationId`. Each exact nested arm requires string
`message`, integer `statusCode`, and boolean `isRetryable`.

Valid types are `authentication_error`, `invalid_request_error`,
`rate_limit_exceeded`, `model_not_found`, `internal_server_error`,
`failed_dependency`, and `forbidden`. `model_not_found` alone may contain exact
`param:{modelId:string}` for the requested public ID. `forbidden` alone may
contain exact `param:{ruleId:string}`. Other params, `code`, `response_error`,
`timeout_error`, and backend debugging fields are invalid. HTTP envelope tests,
not the payload schema, enforce equality between HTTP status and `statusCode`.
The explicit wire boolean is authoritative for a future Go client; pinned
Gateway derives its own retryability from HTTP status.

## Go representability

The future strict adapter cannot marshal the existing provider structs directly:

- `omitempty` collapses absent versus explicit empty arrays/maps, false, zero,
  and empty strings.
- Existing flat union structs can hold inactive siblings.
- Local provider metadata permits scalar or null namespace values.
- Local response metadata includes a `provider` member absent upstream.
- Local file data can serialize an unselected empty object.
- Local timestamps and source fields do not preserve every wire-presence rule.

No intrinsic wire value is unrepresentable in Go, but a future runtime needs a
presence-aware strict DTO/decoder boundary and explicit domain adaptation. H1
does not create it. If a later adapter cannot preserve a distinction in this
ledger, implementation must stop rather than open a standard object or silently
normalize the value.
