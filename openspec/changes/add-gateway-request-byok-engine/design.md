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
- Construct request-local native Anthropic/OpenAI models from only supplied API
  keys, fixed endpoints and credential-independent transport; disable redirects
  and native retries. Validate all supplied entries, including unused providers.
- Bound selectors at 2,048 bytes, keys at 4,096 bytes, arrays at eight credentials
  and the raw BYOK subtree at 65,536 bytes.
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
