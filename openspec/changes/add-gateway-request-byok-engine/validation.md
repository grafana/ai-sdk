# Engine evidence

## Validation

Validated on this branch, independently of later stack changes. All gates were
rerun after rebasing onto main at `d863b86e` (#326/#328). Catalog selection now
retains opaque native options rather than restoring the deleted policy inventories.
All gates below also passed after rebasing onto reviewed PR #367 at `58487635`,
including its capture simplification, consolidated tests and synchronized/archived
client specs. Range-diff confirmed the three engine commits reapplied unchanged.
This engine change remains active for review; the activation branch is untouched:

- `mise run test-short`, `build`, `vet`, `lint`, `parity-check`.
- `mise run test-ai-gateway-source-integration` and `test-integration`,
  including pinned-client typechecks and real-command tests.
- `mise run lint-docs`, `verify-ai-gateway-boundary`,
  `verify-sdk-gateway-isolation`, `verify-merged-pins`, `test-ci-workflow`.
- `mise run test-ai-gateway-image-source` (not skipped) and
  `build-ai-gateway-image`.
- SDK race tests for Grafana, logger, Agent Observability, enrichment and fallback;
  Gateway race tests for command internals and ProviderWire.
- Explicit provider-shape comparison against installed registered
  `@ai-sdk/provider@4.0.18` source; all discriminator families match.
- Strict OpenSpec validation and `git diff --check`.

Review consolidation keeps the new BYOK package's tests in model_test.go and
moves selection cases into the existing handler suite. Test bodies are unchanged.
BYOK/ProviderWire race tests, vet, lint, parity and source integration passed after
consolidation; production code and other stack branches were untouched.

## Review cleanup

All validation gates above passed again after simplifying selection/cancellation
and sharing the native-option guard. The command suite passed all 57 cases and
the image-source regression ran without skips. Strict OpenSpec 1.14.0 validation
also passed after updating the selection contract and removing a stray read marker.

Before the main integration below, a failing BYOK regression demonstrated native I/O for Anthropic MCP
servers/history, skills and server-side fallback. Unary/streaming tests now prove
zero native calls for those controls, with ordinary-option acceptance. The
configured validator/wrapper moved unchanged apart from package/exported names
into internal/nativeoptions; configured compatibility checks remain covered by
the existing service suite. BYOK applies the Anthropic guard outside fallback.

Selection now returns only model and identity; the handler retains mapped native
options. Unary invocation has one cancellation authority while retaining child
cancellation, buffered completion and panic containment. Unused account-access
and BYOK-discovery errors are deferred to activation.

PR #369 was not changed. Its eventual rebase must own those activation errors,
stop returning Selection.Options and remove its duplicate Anthropic guard now
provided by the engine.

## Main integration

Rebased onto PR #367 at `6f00c0e8`, which merged origin/main `8d36b82d`.
Provider-tool unresolved-history mapping now runs before selection and remains
available to unary and streaming output, alongside the shared execution deadline.
Opaque metadata and native MCP support from main remain intact.

The shared native-option implementation matches main byte-for-byte apart from
package/exported names. Its private MCP bounds test moved unchanged into the
nativeoptions package rather than exposing validation internals. New main test
call sites use CatalogSelector.

BYOK tests now follow main's MCP contract instead of the earlier blanket refusal:
valid configuration and matching provider-owned history reach native I/O, local
MCP-looking markers remain inert, and invalid destinations/unconfigured history,
skills and native fallback fail before attempts. Unary and streaming tests
continue asserting the fixed inference host/model and request-supplied key.

All listed build, test, race, vet/lint, parity, integration, boundary, workflow,
pin, provider-shape and image gates passed again; the command suite passed 74
tests. Strict OpenSpec 1.14.0 validation passed for this active change. The two
unchanged main-spec Purpose warnings recorded in PR #367 remain outside this
merge/rebase. Registered upstream package versions are unchanged.

PR #369 remains at `4e5bd486`; it has not been rebased or pushed.

## Typed construction review (superseded decoding policy)

The first construction cleanup returned an opaque validated Request from
DecodeRequest; New received no raw JSON. The account-configuration review below
supersedes that opaque type and keys-only policy.
Credential decoding retains exact case-sensitive fields, byte/count bounds,
last-member-wins semantics and validation of unused entries. Added regressions
cover replaced malformed duplicates, credential endpoint/header rejection,
zero requests, missing transports and ownership after clearing raw input.

Configured and BYOK paths share the small nativemodel constructors. That package
imports only native adapters/SDKs, the provider contract and net/http, with no
catalog, configuration or secret resolver. Configured endpoints remain explicit;
BYOK still uses only fixed defaults. Existing native HTTP, environment poisoning,
retry, redirect, content, MCP and concurrent-isolation tests pass through the
shared construction path. Service factory tests also assert explicit endpoint
and transport forwarding.

All listed validation gates passed again, including 74 command tests, full SDK
and Gateway race suites, parity and image checks. Exact OpenSpec 1.14.0 validation
passed for this existing change; no new OpenSpec change was created.

That cleanup required a later PR #369 selector migration; the account review
below records the current API and destination-policy integration requirements.

## Account configuration review (superseded decoding policy)

Removed the opaque request and custom credential unmarshaler. JSON-schema
validation now owns provider-specific shape/exact names, plain Config values
carry accounts, and construction accepts provider/model/ordered accounts.
Duplicate normalization preserves replaced malformed members before typed
decoding; raw BYOK bytes and all unused accounts remain validated.

Accounts accept baseURL with native defaults and exact provider-scoped service
approval for custom destinations. Only OpenAI accepts organization/project;
empty/omitted optional values retain native endpoint/unset account defaults.
The approval map supplies permission only, never endpoints/accounts/defaults.
It is a separate host argument, not request metadata or configured provider
lookup. Clients cannot set retries, transport/TLS, redirects, arbitrary account
headers or deadlines.

A red regression first demonstrated rejection of explicit native baseURL,
organization/project and empty optional defaults. Green unary/stream tests
verify native headers and body separation, per-account endpoint/header changes
through ordinary fallback, zero native retries, and concurrent account isolation.
Policy tests cover provider-specific approvals, absent approval, path/port/prefix
mismatches, forbidden URL forms, null/unknown/case-variant fields, byte ceilings,
invalid unused entries and last-member-wins behavior. Native-default and approved
endpoint redirect tests both reject redirect following without mutating clients.

Pinned @ai-sdk/gateway 4.0.96 and independent Go-client capture tests preserve
extended account fields/order and caller metadata. Actual logger sinks redact
the entire extended account subtree across generate/stream/error and truncation.
This establishes client projection, not Vercel hosted-service configuration
semantics. Native OpenAI field names/defaults match registered @ai-sdk/openai
4.0.78; no ambient account defaults are inherited.

All independent gates above passed, including all 74 command tests, SDK/Gateway
race suites, parity, integration, typechecks, provider-shape and image checks.
Strict OpenSpec 1.14.0 validation passed for the existing change.

PR #369 must later pass a service-owned approval map to DecodeRequest, call New
with the resulting Provider/Model/Accounts, and expose/validate operator approvals
separately from configured accounts. That activation/config/deployment work is
not implemented here. Approval is trust in an exact endpoint, not a claim of
DNS pinning, public-IP enforcement or deployed proxy/network isolation. The
deployment owner must control approved destinations and their network boundary.
PR #367 and PR #369 remain unchanged.

No module pins, upstream baseline or authentic provider fixture inputs changed.
Local fake services and image tests do not establish live native acceptance,
Vercel hosted-service behavior or deployed Cloud/network authorization.

The command and config validator use `CatalogSelector`; no `NewBYOKSelector`
is installed. Existing command cases still exercise the old modes and catalog.
The new engine has focused native HTTP/race tests, while selector tests prove
one logical invocation and a budget shared with selection. Service activation,
operator BYOK capture and deployed policy remain the next change's responsibility.

## Decoder simplification review

The operator-approved simplification supersedes the schema/normalization and
byte/header/URL-grammar policies in the preceding review sections. DecodeRequest
now uses a Gateway-control map containing provider-to-Config maps, standard Go
JSON decoding and DisallowUnknownFields for account structs. It checks supported providers, 1–8 accounts
per provider (including unused entries), nonempty keys, OpenAI-only nonempty
organization/project, selected-provider availability and exact endpoint approval.
The control name remains exactly lowercase byok, matching both supported
clients and the existing logger capture boundary. BYOK and mixed-case controls,
alone or alongside byok, fail. Account fields use canonical client names without
adding acceptance tests or compatibility guarantees for incidental decoder
case matching. Null and duplicate members follow ordinary Go semantics,
including rejecting malformed earlier duplicates and replacing map values. The obsolete internal unsupported-control sentinel was removed;
unknown controls still fail with a non-secret invalid-request diagnostic.

Removed the account schema, generic JSON round trip, per-field/subtree/selector
byte ceilings and custom header/URL grammar. The existing ProviderWire request
bound (1 MiB command default) applies before selection. Native HTTP transport
validates outgoing headers. The service must validate its approved destination
policy once when loading it; engine requests only check exact membership.
That configuration remains activation-owned and is not implemented in this PR.

Red regressions failed against the previous decoder before implementation.
Green tests cover ordinary account decoding, exact control names, account
cardinality, values larger than previous byte ceilings and endpoint approval
without bypass. Separate red regressions proved
that the initial struct envelope accepted uppercase BYOK; the final map envelope
rejects it without changing shared capture code. Unary/streaming wire tests prove the complete request boundary and zero
selection/invocation above it. TLS test servers plus real HTTP transports prove
invalid API-key/organization/project headers fail before any request is sent.
Existing native-content, account isolation, fallback, MCP and redirect tests pass.

All independent gates listed above passed again: build/test/vet/lint, parity,
SDK/Gateway races, integrations, docs, boundaries, pins, CI workflows, registered
provider-shape comparison and image checks. The command suite passed 74 tests
with no skips. Strict OpenSpec 1.14.0 validation passed for this existing change.
Full logs: /tmp/byok-stack-pr2-simple-decoder/. Focused red/green/race logs:
/tmp/byok-decoder-{red,green,race}.log and
/tmp/byok-canonical-control-{red,race}.log. After aligning tests/specs with the
canonical-casing instruction, focused races, docs lint and strict change
validation passed again; final logs are /tmp/byok-decoder-final-{race,docs}.log.

Registered Gateway 4.0.96 projection and its generic account-record shape remain
the upstream reference; no private-service decoding policy is inferred from it.
Standard Go decoding is an intentional service adaptation, not a claim of Vercel
hosted-service malformed-input semantics. No pins or provider recording inputs
changed. PR #367 and PR #369 remain unchanged; this change remains active.

## Client-guide usability review

Rewrote docs/providers/grafana-gateway.md around authentication, model selection,
generation, provider settings, tools, BYOK, credential-safe logging, fallback,
continuation and error handling. Removed engine/PR-stage details, protocol
encoding inventories and cross-language decoder comparisons. Retained the
user-visible BYOK availability prerequisite, Gateway authentication, exact
custom-endpoint approval and safe retry/fallback/logging guidance. Existing
inbound headings for discovery, fallback and response values remain intact.

All four Go snippets compile together against the public workspace SDK/client
APIs. Both TypeScript snippets pass strict type checking with registered ai
7.0.118 and @ai-sdk/gateway 4.0.96, including the existing configured-discovery
helper. Prepared sources and the Go build are under
/tmp/byok-doc-examples.Bewb9i/. These were compile checks without provider calls.

mise run lint-docs passed structural/link and markdown checks;
mise run validate-parity-baseline passed, including 109 tooling tests. Strict
OpenSpec 1.14.0 validation passed for this existing change. No runtime code,
module pins, provider recordings or other stack branches changed; earlier
runtime validation remains the behavioral evidence.
