## Context

`2026-04-30-lossless-provider-wire` deliberately made the provider wire a
Go-to-Go internal format. Two of its decisions diverge from upstream
`LanguageModelV4` JSON on purpose:

- **D6**: `system` messages serialize as `{role:"system",content:[{type:"text",
  text:"..."}]}` (uniform `[]ContentPart`), not upstream's `{role:"system",
  content:"..."}`.
- **D4**: the error stream part uses `apiCallError`, not upstream's `error`.

Its Non-Goals also say cross-language wire is out of scope. A PoC
(`poc/gateway-interop/`) confirmed the upstream `@ai-sdk/gateway` client already
interoperates for transport, headers, SSE framing, stream parts (`text-*`,
`response-metadata`, `finish`), `finishReason` (`{unified,raw}` — matches), and
`usage` (nested — matches). The only request-path blockers are three inbound
shape mismatches: `system` content (string), tool-result `output.value`, and
file `data` (tagged union).

This change adds **inbound decode tolerance** for those shapes without changing
the canonical emitted form, so we get upstream-client interoperability while
fully preserving the Go-to-Go wire (D6/D4 emitted shapes unchanged).

## Goals / Non-Goals

**Goals:**

- The `provider` decoders accept upstream `LanguageModelV4` JSON for `system`
  message content, tool-result output, and file data.
- Go↔Go wire bytes are unchanged (encoders untouched) ⇒ no conformance
  regeneration; existing round-trip tests stay green.
- Regression tests lock both the canonical round-trip and the upstream-shape
  decode.

**Non-Goals:**

- Changing what Go emits (D6/D4 canonical serialization stays).
- Full two-way cross-language fidelity for the *response* path (mid-stream
  `error` part stays `apiCallError`; provider-executed tool-result/file parts in
  responses stay Go-shaped). Those are not on the request path a gateway client
  sends and can be addressed later if needed.
- File-data `reference`/inline-`text` union variants and file parts nested
  inside tool-result `content` (documented as known gaps).

## Decisions

### D1. Tolerant decoders via custom `UnmarshalJSON`, encoders unchanged

Add `UnmarshalJSON` to `Message`, `ToolResultOutput`, and `DataContent`. Each
detects the incoming shape and normalizes into the existing Go struct. No
`MarshalJSON` is added, so serialization is unchanged.

- `Message.UnmarshalJSON`: decode `role`/`providerOptions` normally; inspect
  `content` — if it is a JSON string, wrap into
  `[]ContentPart{{Type: text, Text: s}}`; otherwise decode as `[]ContentPart`.
- `ToolResultOutput.UnmarshalJSON`: decode the canonical split fields; if a
  `value` field is present, map it by `type` (`text`/`error-text` → `Text`,
  `json`/`error-json` → `JSON`, `content` → `Content`).
- `DataContent.UnmarshalJSON`: if the object has a `type` field it is the
  upstream union (`data` → `Base64`, `url` → `URL`); otherwise decode the
  canonical `{bytes|base64|url}` fields.

**Why:** additive, backward-compatible, localized to three types. Keeps a single
canonical emitted form (no drift, no fixture churn) while accepting the upstream
dialect on the boundary the hosted server actually receives.

**Alternative — symmetric encode+decode change (my findings' Option 2):**
rejected here because it changes Go↔Go wire bytes, contradicts D6/D4's emitted
form, and forces conformance regeneration for no request-path benefit.

**Alternative — server-side adapter in `grafana-assistant-app` (Option B):**
rejected as the primary because the fix then lives outside the shared wire and
the Go client and upstream client would exercise different decode paths; the
shared-decoder approach makes every wire consumer tolerant uniformly.

### D2. Backward-compatibility guardrail

Custom `UnmarshalJSON` must accept the canonical shapes byte-for-byte so the
existing `TestCallOptions_WireRoundTrip` (`assert.Equal(full, decoded)` over a
system message, split tool-result, and `{url}` `DataContent`) stays green. This
is the executable contract that "the wire keeps working."

## Risks / Trade-offs

- **Ambiguity between legacy and upstream shapes** → mitigated by disjoint
  discriminators: canonical `DataContent` has no `type`; upstream union always
  has `type`. Canonical tool-result has no `value`; upstream always has `value`.
  Canonical message `content` is an array; upstream `system` is a string.
- **Partial response-path fidelity** (error part, provider-executed tool
  results/files) → documented gap; request path (what a gateway client sends) is
  fully covered, which is the interop use case.
- **Silent drops for unsupported union variants** (file `reference`/inline
  `text`, files nested in tool-result `content`) → documented; can be added if a
  real client needs them.

## Migration Plan

Additive; no migration. Land decoders + tests together. Rollback = revert.
