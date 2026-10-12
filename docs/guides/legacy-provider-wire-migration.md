# Upgrade providers behind a legacy ProviderWire endpoint

Applications that still import `github.com/grafana/ai-sdk/gateway/providerwire`
must migrate or isolate that transport before upgrading the SDK. A newer
provider module can select a newer core module that no longer contains the old
handler, even when the application still requires an older core version.

## Historical compatibility boundary

[The removal in #94](https://github.com/grafana/ai-sdk/pull/94) immediately
follows commit `bc004f46917f7dc102c330dd01899cf819bc7e94`. The exact historical
versions are:

| Module | Last snapshot before removal |
| --- | --- |
| `github.com/grafana/ai-sdk` | `v0.1.0-alpha.1.0.20260909164941-bc004f46917f` |
| `github.com/grafana/ai-sdk/providers/bedrock` | `v0.0.0-20260909164941-bc004f46917f` |

These are commit-derived Go pseudo-versions, not a newly published compatibility
release. The old Bedrock snapshot lacks the reasoning fixes in
[#247](https://github.com/grafana/ai-sdk/pull/247): decoding Converse
`redactedContent`, replaying that field under its correct name, and preserving
present-but-empty reasoning metadata.

The separate
[frozen legacy module](https://github.com/grafana/ai-sdk/blob/a6f0c2f8d70d1e06f7ea401928c0c1f0629fd66e/legacy/README.md)
copies the old core under `github.com/grafana/ai-sdk/legacy`. Its README states
that it exists only on `legacy/provider-wire` and is not developed. It is a
frozen transport/core snapshot; it does not include the nested Bedrock provider
module or establish a provider-fix backport line. Consumers needing later
provider fixes must move their provider calls to the current modules. Pin the
legacy snapshot exactly; do not use `@main` or `@latest` for it.

## Why upgrading only Bedrock fails

Go selects one version per module path. Bedrock at the #247 commit requires
core `v0.1.0-alpha.1.0.20260921202550-3dff0f7087dc`, which is after the handler
removal. Adding that provider therefore raises an older application's core
selection and leaves its `gateway/providerwire` import unresolved.

Pinning the historical `providers/grafana` client separately cannot restore the
missing package: that client itself imports `gateway/providerwire`. The current
[`providers/grafana` client](../providers/grafana-gateway.md) implements strict
ProviderWire V4; sharing the historical module path does not make it a drop-in
client for the old tolerant endpoint.

The provider's transitive core requirement is also not sufficient evidence of a
compilable version set. In an independent module, the #247 Bedrock version with
only its transitive core pin fails to compile against newer file-input APIs.
Select and verify the core explicitly as well as the provider.

## Isolate the historical boundary

Prefer migrating callers and the endpoint together to the current Gateway
contract when possible. If the old endpoint must remain temporarily, keep its
types and codecs inside an application-owned adapter:

```text
legacy caller → frozen handler + legacy/provider types
                              ↓ explicit application translation
                  current provider types → current Bedrock
```

1. Inventory both direct imports of the retired package and dependencies that
   still import it, including a historical `providers/grafana` client. Remove or
   isolate those dependencies before upgrading the core. A dependency's
   internal imports do not change when the application's imports change.
2. Pin the frozen module and a tested core/provider pair. The following exact
   set resolves #247 for Bedrock Converse and permits both type namespaces in
   one build:

   ```bash
   GOWORK=off go get \
     github.com/grafana/ai-sdk/legacy@v0.0.0-20261006065905-a6f0c2f8d70d \
     github.com/grafana/ai-sdk@v0.1.0-alpha.1.0.20260929203603-566db5d9df4f \
     github.com/grafana/ai-sdk/providers/bedrock@v0.0.0-20260929203603-566db5d9df4f
   ```

   This is an exact migration reference, not a promise that other modules or
   newer provider versions compile with the same core. Choose a newer tested
   set when adopting later fixes.
3. Move handler-side imports to the frozen namespace. Keep provider execution
   in the current namespace:

   ```go
   import (
       legacywire "github.com/grafana/ai-sdk/legacy/gateway/providerwire"
       legacyprovider "github.com/grafana/ai-sdk/legacy/provider"
       "github.com/grafana/ai-sdk/provider"
       "github.com/grafana/ai-sdk/providers/bedrock"
   )
   ```

   `bedrock.New(...)` implements current `provider.LanguageModel`. It cannot be
   returned directly from a resolver requiring `legacyprovider.LanguageModel`:
   their `CallOptions`, results, stream parts and errors are distinct Go types.
   The adapter must implement the legacy interface and translate the supported
   request and response fields around calls to the current model.
4. Define the adapter's supported subset explicitly. Preserve IDs, ordering,
   usage, finish reasons, provider options/metadata, cancellation, error status
   and retryability for that subset. Reject unsupported fields or events rather
   than dropping them. A whole-struct JSON round-trip does not prove semantic
   compatibility, especially for presence, generated files and typed errors.
5. Keep provider metadata in its original namespace and preserve its raw JSON
   values. For decoded `RawProviderOption` values, translate their `Raw` payload,
   not the wrapper's `Key`/`Raw` struct. Do not copy Anthropic redacted data into
   Bedrock metadata as a substitute for receiving Bedrock's own continuation
   data.

No general legacy adapter is supplied by the current SDK. The frozen module
keeps historical codecs separate; the application's translation and endpoint
remain its own migration responsibility. Do not use `replace` to force a newer
provider onto the old root: it conceals the module-selection problem without
making newer provider APIs available.

## Verify the consumer boundary

Run checks in the application's independent module so a workspace does not
silently substitute source for published dependencies:

```bash
GOWORK=off go mod tidy
GOWORK=off go list -m github.com/grafana/ai-sdk \
  github.com/grafana/ai-sdk/legacy github.com/grafana/ai-sdk/providers/bedrock
GOWORK=off go build -mod=readonly ./...
GOWORK=off go test -mod=readonly ./...
```

Test the actual old endpoint, translation, current provider and outgoing
Bedrock request together. A synthetic Converse stream containing
`reasoningContent.redactedContent` must retain its opaque data in the legacy
response; submitting that history again must send
`reasoningContent.redactedContent`, not `redactedReasoning`. Also test empty
signatures, every tool/file feature the application serves, stream failures and
cancellation. Successful compilation alone does not establish that coverage.

The exact version set above was checked in an independent module with no
workspace or local replacements. A focused synthetic HTTP test exercised the
frozen handler, a limited application translation, Bedrock streaming decode,
redacted replay and a present-empty signature. That evidence covers this
reasoning path; it does not establish a complete legacy transport bridge,
Mantle behavior, compatibility with a historical Grafana client, live AWS
acceptance, or adoption by a deployed consumer. Existing stored responses that
already lost opaque reasoning cannot be repaired by upgrading the decoder.

---

[Prev: Amazon Bedrock](../providers/bedrock.md) · [Docs index](../README.md) · [Next: Grafana AI Gateway](../providers/grafana-gateway.md)
