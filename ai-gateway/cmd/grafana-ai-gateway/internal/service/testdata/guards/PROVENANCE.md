# Guard codec evidence

The four sibling files `README.md`, `request-preflight.json`,
`request-postflight-guard.json`, and `responses.json` are byte-identical copies
from `/Users/alexander/projects/agento11y/conformance/hooks` at commit
`9ab60bc5cdadaebc8cd0b3d5cbdb3b23b498f652` (clean fixture directory).
The imported README describes their source-check and local server round-trip
provenance. These are hook wire witnesses, not recorded provider traffic or
proof of deployed Sigil compatibility.

SHA-256:

- README.md: `0feea3f6ed60bb0da0a16363d6e31f50cde2957ce4ebf1b1c1a4624c6d3255c8`
- request-preflight.json: `54bf35f1a475bb0b5025b6402a0b100ea4e342e7be939efc9061407a870c9369`
- request-postflight-guard.json: `959ebed8d2ac56b9b06d934014dea197f7dee9429be7f3372129fedc7f772f0e`
- responses.json: `12880fe2e189aea721fb233fe5db3a97f003dea1f63cde687fe96009bac6df22`

Contract checked against Sigil commit
`de1631e8902b2c3e94202022ab3e88136607d3ec`:

- `sigil/internal/eval/hooks/http.go`: `wireHookInput`,
  `toHookGenerationInput`, `protoPartsToWire`, and `protoMessagesToWire`.
  Requests embed tool arguments/results as JSON. Tool schemas and response
  tool JSON use base64-encoded byte fields. System message roles are accepted
  and emitted as strings.
- `sigil/internal/eval/hooks/service.go`: `Evaluate` evaluates both phases,
  takes postflight from `input.output`, and snapshots complete transformed input.
- `sigil/internal/eval/hooks/transform.go`: `ApplyTransform` redacts text,
  descriptions and tool JSON, preserves thinking, and can collapse JSON to
  `[REDACTED]` or `{"redacted":"[REDACTED]"}`.
- `middleware/agentobservability/hooks.go` was read as mapping precedent only;
  its SDK normalization and inverse matching are deliberately not reused.

The Go tests additionally use explicitly synthetic in-memory codec inputs.
They do not manufacture provider recordings or treat request echoes as evidence
for the asymmetric response encoding. The imported response is decoded
independently; its reduced message/tool snapshot is not a safe replacement for
its corresponding full preflight fixture.

Registered upstream: `ai` 7.0.118, `@ai-sdk/gateway` 4.0.96,
`@ai-sdk/provider` 4.0.18, commit
`5d12eaa6caa193d3901cbab98a734403eb6bf622`.
The installed exact-version provider source (`language-model-v4-call-options.ts`
and `language-model-v4-prompt.ts`) was inspected. The coverage classification is
Gateway/provider-contract extension, not Vercel private-service parity. No
provider interface or public ProviderWire change is made by this codec.

## Deliberate codec boundary

Supported: positional system/user/assistant/tool messages, text, represented
assistant reasoning, function definitions, consumer-executed tool calls, text
and JSON/error results, execution-denied reasons, and nested text-only results.
System messages stay in `messages` with their original role and part boundaries;
no lossy joined `system_prompt` or derived `conversation_preview` is generated.
The only provider metadata accepted is a raw Anthropic reasoning signature with
nonempty represented thinking, preserved unchanged and never sent as content.

All opaque options at other scopes, headers except `User-Agent`, stop sequences, schema/name/
description-bearing response formats, input examples, provider tools, stored
references, media/custom/source/approval parts, and populated inactive union
fields are rejected. Scalar generation controls, content-free response format,
function strictness and valid tool choice survive transforms unchanged.

Transforms preserve message/part/tool count, order, role, kind and exact tool
identity/error status. Thinking cannot change. JSON rewrites conservatively
allow string value changes with unchanged keys/structure and exact rational
numeric equality, plus Sigil's whole-object/array redaction collapse. Structural
rewrites other than those collapses fail closed rather than replay original
content. Byte payloads must be canonical padded base64 of strict JSON; no SDK
fallback normalization is used. Duplicate keys, invalid Unicode and overly deep
or extreme numeric JSON are rejected. Every postflight transform is unusable;
HTTP 424 enforcement belongs to the runtime rather than the codec.

No live policy, ingress authorization, HTTP runtime, streaming enforcement,
resource capacity or matching evaluator coverage is established here.
