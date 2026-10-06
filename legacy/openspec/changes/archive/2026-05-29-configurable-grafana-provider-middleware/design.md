## Context

The Grafana provider (`providers/grafana`) is a transparent provider-wire
transport. It encodes `provider.CallOptions` to JSON, sends them to
`POST <BaseURL>/language-model`, and streams back `provider.StreamPart` values.
It runs no middleware itself.

The built-in middlewares the clients want to influence live in
`grafana-assistant-app`, composed server-side around each catalog model in
`api/internal/aisdkplatform.NewClaudeModel` via `aisdkmiddleware.WrapLanguageModel`.
Today only `LoggerMiddleware` is wired; sigil, tracing, metrics, and usage
tracking are planned additions. The provider-wire `Handler` decodes the request
body into `provider.CallOptions` and calls `DoStream`/`DoGenerate` on the wrapped
model, so anything inside `CallOptions` is already available to every server-side
middleware without Handler changes.

The repo already has a typed-options machinery (`typed-provider-options` spec):
`provider.ProviderOption` (interface with `ProviderKey() string`),
`provider.ProviderOptions` (`map[string]ProviderOption` with lossless JSON),
`BuildProviderOptions`, and `ResolveOption[T]`. `AnthropicOptions` is the
existing precedent for a typed, namespaced option that round-trips over the wire.

The server side already models graded sigil capture via
`sigil.ContentCaptureMode` (`full`, `metadata_only`, `full_with_metadata_spans`)
and resolves a per-tenant default through `sigilregistry`/`tenant/limits`.

Decisions taken with the requester:
- Sigil control is graded (capture mode), not boolean.
- Client preference wins over the server-side tenant default.
- Per-middleware control structs, not a unified knob.
- Attach via `CallOptions.ProviderOptions` (`WithProviderOptions`), not a context helper.
- `GrafanaOptions` lives in `providers/grafana`; the backend imports it.
- Unknown/invalid fields fail the request.

## Goals / Non-Goals

**Goals (this change, ai-sdk repo):**
- Define a typed `GrafanaOptions` option, namespaced `"grafana"`, that clients
  attach per request.
- Express per-middleware control explicitly and self-documenting (sigil, tracing,
  metrics, usage), with sigil graded by capture mode.
- Round-trip losslessly over the existing provider-wire `providerOptions` channel
  with no new headers and no Handler changes.
- Validate known fields client-side and surface invalid values before sending.
- Define the contract semantics (client-preference-wins, disable-suppresses,
  strict rejection) so the follow-up backend change implements against a clear
  spec.

**Non-Goals (deferred to the `grafana-assistant-app` follow-up PR):**
- Backend resolution of `GrafanaOptions` from request `CallOptions`.
- Server-side strict unknown-field rejection and 4xx error mapping.
- Client-preference precedence enforcement over tenant defaults in each middleware.
- Implementing the server-side sigil/tracing/metrics/usage middlewares
  themselves; they consume the contract as they are added.

**Non-Goals (this change, permanent):**
- Any change to provider transport, auth, constructors, or the
  `aisdkprovider.Handler`.
- A context-based attach API or a unified enable/disable flag.
- Server-side enforcement that overrides client preference (the contract is
  client-authoritative; tenant policy precedence may be revisited later).

## Decisions

### D1: Carry control via `ProviderOptions`, not a context helper

`CallOptions.ProviderOptions` is the ai-sdk-native channel for provider-specific
call parameters and is already serialized into the wire body and decoded into
`CallOptions` server-side. Using it means zero new transport plumbing on the
client and zero Handler changes on the backend; middlewares read
`opts.ProviderOptions` directly.

Alternative considered: a `grafana.WithOptions(ctx, ...)` context helper
mirroring `WithUserIDToken`. Rejected because `WithUserIDToken` carries transport
auth, not call params; a context helper would force the provider to merge
context-supplied options into `CallOptions` before encoding, introducing a
merge-precedence problem and two competing ways to set the same data.

### D2: One namespaced option with per-middleware sub-structs

`GrafanaOptions` implements `provider.ProviderOption` with
`ProviderKey() == "grafana"`. It holds one pointer field per controllable
concern:

```
type GrafanaOptions struct {
    Sigil   *SigilControl
    Tracing *TracingControl
    Metrics *MetricsControl
    Usage   *UsageControl
}
```

Pointer fields make "not set" (nil) mean "backend default applies", distinct from
an explicit value. Each middleware reads only its own sub-struct, so concerns are
independently controllable and the API is self-documenting. Every control struct
carries a `Disabled *bool` (D3a); middlewares whose only knob is on/off start as
`{ Disabled *bool }` and grow graded fields later without changing the shape.

Alternative considered: a unified `Disable []string` or `Sensitive bool` knob.
Rejected per requirement #3 -- it hides intent and leaks no type-level
documentation of what each toggle does.

### D3a: Hard-disable knob on every control struct, distinct from graded config

Each control struct SHALL carry a `Disabled *bool`. When true, the corresponding
server-side middleware MUST short-circuit and run none of its behavior for that
request -- for sigil, it skips `StartGeneration` entirely so no generation record
is produced. This is orthogonal to any graded config on the same struct:

- `Disabled` suppresses the middleware's *event* entirely (no record at all).
- A graded field (e.g. sigil `CaptureMode`) shapes the *payload* of an event that
  still happens.

This distinction is necessary because the sigil SDK's `ContentCaptureMode` has no
"off" value: even its most restrictive mode (`MetadataOnly`) still emits a
generation record carrying structure, tool names, usage, timing, and unstripped
user `Metadata`/`Tags`. A client wanting "do not track this sensitive
conversation at all" needs the hard-disable gate, not a capture mode. The
short-circuit mirrors the existing client-side `HooksMiddleware`/`RecordingMiddleware`
`Enabled func(ctx) bool` gate pattern.

When both `Disabled: true` and a graded field are set on the same control,
`Disabled` wins -- there is no event for the graded field to shape.

Alternative considered: a single `GrafanaOptions`-level `Disabled map[string]bool`
keyed by middleware name. Rejected for the same reason as the unified knob in D2:
stringly-typed, less discoverable, and inconsistent with the per-middleware
typed-struct approach.

### D3: Graded sigil control mirroring `ContentCaptureMode`

`SigilControl` carries, in addition to the `Disabled *bool` from D3a, a
`CaptureMode` typed string enum whose values mirror the
server-side `sigil.ContentCaptureMode` set. The client expresses intent (e.g.
"metadata only" for a sensitive conversation); the backend maps it to the sigil
SDK value. Following the repo's `typed-string-enums` convention, `CaptureMode` is
a named string type with typed constants, never a bare string.

Alternative considered: a boolean `Disabled`. Rejected per requirement #1 -- the
server already supports graded modes and a boolean would discard that capability.

### D4: Backend resolves via `ResolveOption`, client preference wins (follow-up)

This defines the contract the follow-up `grafana-assistant-app` change
implements; no backend code lands in this ai-sdk change. Each server-side
middleware will call
`provider.ResolveOption[GrafanaOptions](opts.ProviderOptions, "grafana")` and, if
its sub-struct is present and set, use the client value in place of the tenant
default resolved from `sigilregistry`/`limits`. Precedence is a per-middleware
one-liner; the contract is "middleware authors opt in by reading their
sub-struct".

### D5: `GrafanaOptions` lives in `providers/grafana`

The type lives next to the provider it configures. The backend already imports
`github.com/grafana/ai-sdk/...` heavily; importing the grafana provider's option
type is a small, honest coupling. After crossing the wire the option arrives as a
`RawProviderOption`, so the backend recovers the typed view via `ResolveOption`
with no shared-contract package needed.

Alternative considered: placing the type in `provider/wire`. Rejected -- `wire`
is transport-only today and has no domain types; adding one would broaden its
role.

### D6: Strict validation, fail the request on unknown/invalid input

Validation has two layers:
- Client-side (this change): `GrafanaOptions.Validate` checks known fields (e.g.
  `CaptureMode` is one of the defined constants) before the call is sent,
  surfacing misuse early.
- Server-side (follow-up, authoritative): on resolution the backend will reject
  unknown control keys and invalid enum values with an `*provider.APICallError`
  carrying a 4xx status (client error, non-retryable). Because client preference
  is honored, a silent fallback would let a client believe it opted out when it
  did not; failing loudly is required.

To detect unknown keys, the follow-up server-side decoding of the `grafana`
namespace JSON uses strict decoding (`DisallowUnknownFields`) so unexpected
fields are errors rather than discarded.

## Risks / Trade-offs

- [Strict unknown-field rejection couples client and backend versions: a newer
  client sending a field an older backend does not know will be rejected.] →
  Treat `GrafanaOptions` fields as a versioned contract; add fields backend-first,
  client-second. Document the ordering in the rollout. The failure is loud and
  diagnosable, which is the intended trade-off over silent drops.

- [Client-preference-wins can conflict with tenant compliance policy that
  requires full capture.] → Explicitly out of scope by decision; the contract is
  client-authoritative for now. If compliance later requires server override, it
  becomes a precedence change in the middleware, not a wire-contract change.

- [Importing `providers/grafana` into the backend adds a module dependency edge.]
  → The option types are small and dependency-light (no authlib, no HTTP); the
  coupling is limited to plain structs and the typed enum.

- [Two repos must agree on the JSON shape under the `"grafana"` key.] → The shape
  is exercised by round-trip tests in `providers/grafana` and resolution tests in
  the backend; the `typed-provider-options` machinery guarantees lossless
  marshal/unmarshal.

## Migration Plan

This is additive and split across two PRs. Existing clients that send no
`GrafanaOptions` are unaffected; `opts.ProviderOptions["grafana"]` is simply
absent and the backend applies its defaults.

**PR 1 -- this change (ai-sdk repo):**
1. Land `GrafanaOptions` and control sub-structs in `providers/grafana` with
   client-side validation, round-trip tests, and README docs.

**PR 2 -- follow-up (`grafana-assistant-app`):** ordered after PR 1 merges and is
synced into the backend's ai-sdk dependency, so the imported `GrafanaOptions`
type is available.
2. Add a resolver helper that reads and strictly validates `GrafanaOptions` from
   `CallOptions`, returning a typed view or a 4xx `APICallError`.
3. As each server-side middleware (sigil first) is added or updated, have it
   consult its sub-struct with client-preference precedence and the disable
   short-circuit.
4. For any future field additions, roll out backend-first to avoid older-backend
   rejection of newer-client fields.

Rollback: PR 1 alone is inert (clients can set options the backend ignores until
PR 2). Reverting PR 1 removes the client option type; because the channel is
additive, behavior returns to backend-default-only.

## Open Questions

- Exact set of controllable middlewares to ship first. The proposal lists sigil,
  tracing, metrics, usage; sigil is the concrete near-term driver. Tracing/metrics/
  usage control shapes can be stubbed in the type now or added per-middleware as
  those middlewares land. Every control struct carries `Disabled *bool` from the
  start (D3a); graded fields are added per concern (sigil has `CaptureMode`).
