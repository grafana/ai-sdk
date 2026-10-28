## Context

The base is #581 at `c9c802b8`, including #555 execution and #579 attempt
visibility. Chat's unary, streaming and compatibility cuts are restacked on
those contracts. This fourth change integrates them with the plugin; it does
not implement the remaining #483 shared provider-lifecycle validator.

The registered Vercel baseline is b7f6255acaae7724adc0fd37aac3dae3f3466578,
ai 7.0.143, provider 4.0.29, OpenAI 4.0.96, Anthropic 4.0.82 and compatible
3.0.71. Native options are compared to that source. The public Chat wire is
consumed by official OpenAI Go v3.76.0 and npm openai 6.27.0. These are distinct
evidence surfaces; Vercel client conformance does not establish Chat behavior.

## Goals / Non-Goals

Expose the represented Chat subset through the existing host, with reliable
client-facing URL/access semantics and cross-language protocol-hop proof.
Do not add a separate access dispatcher, Chat-specific catalog wrapper, private
execution worker/drain/classifier or credential-bearing native SDK extension.

## Decisions

### One runtime and route entry

Gateway construction passes its execution.Runtime to chatcompletions.New.
productionRoutes receives the handler and adds an entry for the relative
`/v1/chat/completions` path, `chat_completions` label and Chat host.ErrorWriter.
The existing registration loop supplies access and telemetry; route matching
and host middleware are not specialized for Chat. The path shim rewrites the
recognized namespaced URL to the codec's relative path, preserving context.
The manifest and route table must agree on all three production operations.

The public endpoint is
`/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/{namespace}/v1/chat/completions`.
Official clients set baseURL to the same prefix ending in `/v1` and use normal
Chat SDK methods. A Grafana service-account token travels as Authorization
Bearer to the platform; a delegated internal token uses the platform's access
token header. No inbound header is reinterpreted as a native provider key.

### Preserve shared access decisions

Self-hosted users/service accounts and cloud allowlisted internal callers can
use configured catalog models. A cloud user/service account classified as BYOK
cannot access that catalog. Chat supplies no gateway BYOK controls, so the
unchanged shared selector rejects that request as invalid BYOK (HTTP 400), with
zero native I/O. This replaces the prototype's dedicated 403 adapter guard.
Actual host account-access refusal still uses the Chat permission document
(403) before reading the body. Do not add a special dispatcher just to force
every refusal into the same status.

### Error and policy ownership

Chat implements host.ErrorWriter and maps execution categories to fixed OpenAI
documents. Host policy rejects credential-bearing call headers and reserved
namespaces, then re-checks translated options at each physical attempt. The
adapter does not inspect raw native failures or replace selection policy.
Errors produced before the plugin, including authentication/oversize/RPC
deadline errors, remain platform documents; official SDK compatibility does
not imply those are OpenAI-shaped.

### Evidence through the plugin protocol

The Go/TypeScript harness builds the real plugin and starts platformhost,
which signs delegated test tokens and forwards over the plugin route protocol.
Synthetic native servers exercise actual provider adapters, translation,
fallback, schema/tool continuation, native identity/warnings and cancellation.
No commercial API or deployed Grafana is involved. Both SDK and plugin phases
must use the explicit candidate go.gateway.work, not standalone module pins.

### Lifecycle and sequential responses

Chat currently uses local representation/lifecycle checks after runtime Next;
#483's generic validator is pending. Runtime owns setup, activity idle, waits,
cancellation and drain already. After one validated native finish Chat requires
closure; any following native part fails without a successful finish/[DONE].
The #583 ProviderWire draft can preserve sequential segments for that protocol,
but Chat must neither silently truncate them nor replay committed output.
Reevaluate this boundary explicitly when the shared validator lands.

## Risks / Trade-offs

Plugin-protocol tests model delegated identities but do not verify real Grafana
credential stripping, route authorization or transport draining. Hosted errors
can differ from Chat errors. Synchronous runtime calls require cooperation with
context; no guarantee is made for permanently blocking native functions or
custom writers. No complete readiness claim is made while #483 is outstanding.

## Migration Plan

Update and test route-table/manifest composition and client harness, validate
the change, sync both capability deltas and archive only after scoped evidence
passes. Keep the PR draft for #483 independently of this change's archive.
