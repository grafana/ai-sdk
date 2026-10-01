## Context

Issue #213 concerns tool-state loss across persistence and resume. The original
reference was `ai@7.0.109` / `@ai-sdk/react@4.0.112` at
`4e8c387622ee1bb0d55841664416d38754d5c9a3`; the merged baseline is 7.0.116 / 4.0.119
at `ee3169b3c4880e2abe4d0d7c781243bb81822ec4`.

Matching `packages/ai/src/` implementations and tests were compared: UI types,
process/read streams, conversion, validation, Agent UI creation, tool-output
projection and provider prompt preparation. Synced specs own the acceptance
contract; `test/conformance/PARITY.md` owns evidence and representation boundaries.

## Goals / Non-Goals

Preserve represented tool/approval state, isolate reader resume, match model
projection and reject invalid history before Agent invocation. Reader lifecycle
APIs (#181), framing (#180), provider redesign, a general exported validator,
application-schema options and blanket optional-scalar migration are out of scope.

## Decisions

- Extend existing static/dynamic tool structs, not a parallel lifecycle model.
  Pointer strings/bools, raw JSON and targeted metadata codecs preserve selected
  presence. `ErrorText` and approval `Reason` become pointers. Reject malformed
  non-null optional fields before decoding can erase them; clone all nested data.
- Keep chunk scalar APIs with narrow decoded-presence tracking. Explicit false
  clears prior provider execution; omission inherits. Direct scalar zero values
  retain omission. Wire approval fields remain `approvalDescriptor` and `reason`.
- Follow static/dynamic transitions: static input errors use RawInput, dynamic
  errors use Input; static error continuations retain RawInput, success clears it.
  Merge approval responses and replace preliminary outputs on one matching part.
- Seed readers with clones at option creation and per invocation. Assistant
  contents resume; non-assistant seeds contribute only ID. Active delta maps stay
  empty and existing progressive/blocking error contracts remain unchanged.
- Filter streaming/preliminary tools before callbacks; preserve nullish input,
  metadata-source and denial semantics. Successful `ToModelOutput` remains intact;
  error, denied and filtered parts bypass it. Data conversion accepts only
  user/assistant text or file results; nil skips and errors abort.
- Use the provider text discriminator to emit required empty text without changing
  `Text string`. Coalesce consecutive tool messages only during prompt preparation,
  preserving order, deep metadata precedence, empty-message participation and
  caller isolation; direct conversion grouping is unchanged.
- Normalize one isolated history before Agent conversion/response assembly using
  `validateUIMessagesForAgent` gates, not public-validator defaults. Check represented
  states and configured static schemas, normalize supported terminal cases, and
  strip non-error dynamic RawInput only from the clone. Dynamic tools skip static
  schemas; history validation does not invoke `Tool.ValidateInput`.

## Evidence / Risks

Root regressions and pinned differentials cover all seven static/dynamic states
and selected absent/empty/false/null values. Schema-parsed SSE and real `useChat`
persist/remount/approval-resume tests check assembled history and the fake provider
prompt. Fixtures are provider-independent, not fabricated provider recordings.

Application/provider schemas not represented by Go, approval `inputSchemaInput`
and schema-transform/refinement paths remain outside validation proof. Existing
scalar/options normalization is explicit, never generic empty-value removal.
Pointer callers migrate together; older binaries can discard new fields, so avoid
read/write downgrades. Required checks include affected modules/examples,
integration, parity and candidate-source CI. Release/adoption still requires
published-module evidence rather than workspace success.
