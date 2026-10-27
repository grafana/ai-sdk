## Context

The registered baseline is `@ai-sdk/anthropic` 4.0.59 at upstream commit
`4e8c387622ee1bb0d55841664416d38754d5c9a3`. Its factory, model and factory tests
establish explicit auth selection, model capability resolution, request model
serialization, and response metadata. It has no Foundry constructor. This is an
intentional Go extension, not a claim of a newly covered upstream surface.

The repository's pinned native `anthropic-sdk-go` v1.75.0 Foundry implementation
and official Anthropic Foundry documentation establish the Azure
`/anthropic/v1/messages` contract, `x-api-key` or bearer authentication, and
credential callbacks per HTTP attempt. The provider does not inject the old
preview `api-version` query or remove the SDK's beta query.

## Decisions

`providers/azure` is a separate module composing the published Anthropic adapter
already used by Assistant. This avoids forcing a core SDK upgrade that removes
Assistant's embedded legacy Gateway transport. Its root and Anthropic module
pins remain published, merged commits. The reference baseline remains unchanged;
compatibility with the older published adapter is tested separately.

The low-level `foundry.Config` exposes explicit endpoint/authentication settings
and reusable SDK request options, including the no-environment-defaults marker
when directly constructing a native client. The older adapter initializes its
client before accepting request options, so the Azure wrapper explicitly
preempts ambient auth and removes environment custom headers before applying
caller options. In callback mode a non-secret credential marker preempts
profile/federation auth; middleware replaces it or aborts before transport.

`azure.NewAnthropic` supplies the canonical model ID to the Anthropic adapter
and changes only the serialized request model to the deployment name. The SDK
provider owns unary and streaming identity normalization, including draining
the upstream channel on cancellation. This is a composition adaptation to the
published adapter interface; applications need no response wrapper.

Secret files, token caching, deployment approval, fallback configuration and
hosting-specific restrictions remain consumer policy. The shared Anthropic
adapter determines available protocol features; this constructor does not
promise parity for features absent from its published dependency.

## Validation and boundaries

Synthetic HTTP tests cover request shape, ambient credential isolation,
per-attempt credential refresh, unary/stream identity, cancellation, and shared
tools/thinking/cache usage behavior. Existing Anthropic fixtures remain unchanged.
No synthetic payload is presented as a live recording; Foundry deployment
acceptance and hosting-version feature availability remain unverified live.
