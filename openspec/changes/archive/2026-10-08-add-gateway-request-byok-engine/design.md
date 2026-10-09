## Approved ordinary-decoding revision

Updated #317 supersedes the strict account-field policy below. Ordinary Go
struct decoding accepts standard casing, duplicates/nulls and additive fields.
Namespace/provider-map keys and discriminator values remain exact; required keys,
provider/account availability and exact destination authorization remain semantic
execution checks. A wire-local modelMappings list marks a requested unsupported
capability: absent/null/empty is inactive, nonempty or malformed is rejected
without inspecting entries. Native Config and constructors stay plain. The
existing eight-account behavior is temporarily retained as execution policy,
with #394/#317 owning its disposition. Current specs/tests reflect this revision;
earlier versioned evidence is historical, not reinstated strictness.

## Context

Stack 2/3 depends on `protect-gateway-client-byok-capture`. The registered
Gateway 4.0.96 / ai 7.0.118 / Provider 4.0.18 reference at
`5d12eaa6caa193d3901cbab98a734403eb6bf622` is unchanged.
Its Gateway client defines BYOK request projection, not hosted-service retry policy.
Native adapters and the existing Go fallback module are the execution reference.

## Goals / Non-Goals

Provide a testable request-only engine and a wire handler that does not require
every selected model to come from a catalog. Preserve native data and existing
namespace/scope-aware native consumption protections. Do not change authentication, discovery policy,
service observers, process modes or listener exposure in this PR.

## Decisions

- Replace `Resolver` with `RequestSelector` and use `CatalogSelector` at all
  current command call sites. Return only model and identity; the handler retains
  mapped options instead of requiring selectors to echo them. Do not introduce a
  compatibility API.
- Extract call-level Gateway controls before ordinary mapping, then pass them to
  the host selector; the catalog adapter still rejects them.
- Start the execution budget before selection, not again at invocation or stream
  startup. Contain panics and bound handler latency even for late selection.
- Decode a Gateway-control map containing provider-to-account maps with typed
  account structs, using the standard Go JSON decoder and DisallowUnknownFields.
  Require the exact lowercase byok control used by both supported clients.
  Use canonical account field names without casing aliases or compatibility
  guarantees. Retain ordinary Go null/duplicate decoding; do not normalize JSON
  or maintain an account schema/custom unmarshaler. Validate supported providers, one to eight
  accounts per provider, nonempty keys and selected-provider availability.
  OpenAI organization/project values remain provider-specific.
- Share small native Anthropic/OpenAI constructors with configured execution.
  They receive explicit account/model/transport values and cannot access
  configuration, catalogs or secret resolution. BYOK construction receives the
  decoded provider, native model and ordered accounts, not raw Gateway JSON.
- Accounts require apiKey and may supply baseURL; OpenAI accounts may also supply
  organization and project. Omitted/empty baseURL uses the native endpoint;
  omitted/empty organization/project stays unset, never inherited from ambient
  SDK state. These account settings are a Grafana service extension, not a claim
  of Vercel hosted-service support.
- Custom baseURL must match an exact service-approved URL for that provider.
  The service owns validation of approved URL syntax when loading its policy;
  request decoding only checks membership. Native defaults remain allowed.
  The approval map authorizes destinations only:
  it supplies no endpoint default, credentials, model mapping or fallback account.
  It is independent of configured catalogs/providers and cannot come from the
  request. No hostname-prefix, wildcard, path-prefix or implicit port matching.
- The engine receives approvals from host composition; operator configuration
  and activation remain PR #369 responsibilities. Approval is explicit trust in
  an endpoint, not a public-address/DNS/proxy isolation proof. The deployment
  owner must control approved destinations, DNS and outbound proxy/network policy.
- Shared transports remain credential-independent; disable redirects and native
  retries. Clients cannot override transport/TLS, retries, arbitrary account
  headers or execution deadlines. Validate unused accounts before execution.
- Reuse the existing ProviderWire whole-request bound (1 MiB command default)
  and HTTP server header limits; do not add BYOK-subtree, field or selector byte
  ceilings. Native HTTP transport owns outgoing header validity. Only account
  cardinality is bounded in the BYOK decoder.
- Reuse `fallback.New` with its default decider, not an additional Gateway
  wrapper or a credential-specific retry algorithm. Preserve native content,
  options and continuation. Share the existing native-option validator/wrapper
  between configured construction and BYOK. Anthropic BYOK validation wraps the
  logical fallback once, preserving bounded MCP configuration/history validation
  from main while refusing skills and native fallback before attempts.

## Risks / Trade-offs

- Non-retryable 401/403 stop; eligible 429/5xx and unknown precommit failures may
  advance. Any first native part commits. Unknown failures can follow paid work;
  this is not exactly-once execution.
- A permanently non-cooperative selector can retain its worker; bounded response
  latency does not imply guaranteed worker reclamation or key zeroization.
- Focused synthetic transports prove construction/isolation, not live acceptance.
- Generic wire errors retain BYOK validation categories, but account-access and
  BYOK-discovery errors belong to service activation. The command does not install
  a BYOK selector or discovery handler in this slice.

## Rebase alignment

Main's #326/#328 removed catalog option inventories and the text-only fallback
wrapper. The catalog selector therefore forwards opaque mapped options unchanged;
native namespace/scope guards remain at consumption, and configured fallback keeps
its mapped capabilities. This change does not restore deleted policy helpers or tests.

## Migration Plan

Update all current handler constructors to the catalog adapter in the same PR.
The next change atomically installs account authorization, BYOK observation and
three listeners. Intermediate commands keep their existing deployment behavior;
they must not be advertised or activated as the new BYOK service.

## Open Questions

None for engine scope. Advanced credential families, routing and public attempt
evidence remain separately owned (#318–#324/#280 and related routing work).
