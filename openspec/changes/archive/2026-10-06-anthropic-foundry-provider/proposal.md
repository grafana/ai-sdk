## Why

Consumers currently implement Azure endpoint/authentication handling and wrap
Anthropic streams to restore canonical identity after deployment-name routing.
The provider should own those reusable semantics while consumers retain
deployment approval, secret storage, and fallback policy.

## What Changes

- Add an Azure provider module composing the published Anthropic provider.
- Share endpoint/authentication options with direct anthropic-sdk-go consumers.
- Separate canonical model capabilities/identity from wire deployment names.
- Document the supported boundary and synthetic test evidence.

## Capabilities

### New Capabilities

- `anthropic-foundry-provider`: Foundry construction, transport, and model identity.

## Impact

New Azure provider module, workspace registration and guides. No frontend wire types, infrastructure,
model approval, residency enforcement, or application startup behavior changes.

## Non-goals

Provisioning Azure resources, storing secrets, managing token caches, enforcing
application deployment allowlists, and claiming live provider parity or residency.
