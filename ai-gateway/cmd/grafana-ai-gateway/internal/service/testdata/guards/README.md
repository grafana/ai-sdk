# Hook wire fixtures

`POST /api/v1/hooks:evaluate` is the request-path guard endpoint. It has no
generated stubs on either side, so these files are the only cross-language contract
for its shape.

| File | Contents |
|------|----------|
| `request-preflight.json` | The preflight request body every SDK must produce: all four part kinds under `input.messages`, two tool definitions, and every context field the server matches rules on. |
| `request-postflight-guard.json` | The postflight body the shipped guards send: one tool call under `input.output`. The server's tool filter scans `input.output` before `input.messages`, so this is the field a tool-filter rule reads in production. |
| `responses.json` | Canned server responses (`allow`, `deny`, `allow_with_transformed_input`) every SDK must parse into the same logical result. |

## The two directions use opposite encodings

`proto/agento11y/v1/generation_ingest.proto` describes neither convention. Both come
from how the server decodes and encodes each field. Each SDK repeats the rule for a
field in the comment next to the code that encodes it.

| Field | Request | Response |
|-------|---------|----------|
| `tool_call.input_json` | embedded JSON | base64 |
| `tool_result.content_json` | embedded JSON | base64 |
| `tools[].input_schema_json` | base64 | base64 |

The server dispatches a part on a snake_case `kind`: `text`, `thinking`, `tool_call`,
`tool_result`. It decodes a part with a missing or renamed discriminator to an empty
part, which is invisible to rule evaluation, so the guard allows the request.

Parsing a response part follows the same dispatch, and all three SDKs drop a part
with nothing to recover: an empty `text`, an empty `thinking`, a `tool_call` or
`tool_result` without its payload object, a `tool_call` without a name, and any kind
an SDK does not know that arrives without `text`. An unknown kind that carries `text` becomes a text part,
because that is the only way the server can have described it.

The kind decides which field a parser reads, and no parser reads a second one. A
`tool_call` without its payload object is dropped even when the part carries `text`,
and a `text` part with an empty `text` is dropped even when the part carries
`thinking`. Recovering the leftover field would report a part the rule never wrote,
and a different `transformed_input` per SDK for one body. A part with no `kind` at
all is the one case a parser reads whichever payload field is set: the server always
sets `kind`, so that shape can only come from a hand-written or protobuf-JSON body.

A message carries its body in `parts`, in both directions. There is no `content`
field on a wire message, so a text shorthand has to travel as a text part.

## Response payloads always parse back as JSON

A response payload is base64 of whatever bytes the proto field held, and nothing
guarantees those bytes are JSON. All three SDKs resolve one string the same way:

1. Strict base64 that decodes to a JSON document becomes that document.
2. Strict base64 that decodes to anything else is kept as a JSON string holding the
   decoded text.
3. A string that is not base64 but is itself a JSON document is kept as is.
4. Anything else is kept as a JSON string holding the original text.

Every SDK has to end up with a valid JSON document. Go holds these payloads in a
`json.RawMessage`, so invalid bytes there fail every later marshal of that part, and
the caller can neither re-export nor re-send the transform. Request serialization
follows the same rule in reverse: an unparsable payload travels as a JSON string, and
the request still goes out.

## What the fixtures deliberately leave out

`ToolDefinition.deferred` is absent, because the JS `ToolDefinition` type has no such
field and adding it would change a public type and the generation-export mapping. Go
and Python cover the field in their own tests.

Media parts are absent, because only Go's `model.Part` can hold one and the server
has no `media` kind. All three SDKs drop a media part, and a message left with no
parts serializes as `"parts": []`.

## Comparison rule

Compare parsed JSON structures, not bytes. Key order and the whitespace inside
embedded JSON are not part of the contract.

This contract allows exactly one normalization, and only the Go suite needs it: drop
`metadata` keys whose value is an empty object, which Go emits because
`model.Part.Metadata` is a value-type struct. Do not normalize anything else. A
different encoding or a renamed key is what these fixtures are here to catch.

## Where the shape comes from

These suites check the SDKs against the fixtures. The shape recorded here came from
reading the server's hook decoder, running both request fixtures through it, and
bringing an equivalent response back out through its encoder. If you change a
fixture, repeat that check first. Do not edit a fixture to make an SDK suite green.
