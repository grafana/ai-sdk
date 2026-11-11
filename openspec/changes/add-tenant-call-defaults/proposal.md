# Proposal

## Why

Org admins can store `Middleware` resources, and the Gateway applies them per namespace, but the shipped Gateway has no tenant extension types, so Grafana refuses every create. Admins also have no way to give every model call in their stack shared instructions or sampling defaults. Two small, off-by-default extensions prove the tenant extension path end to end against Grafana and give admins those defaults.

## What Changes

- **`default-instructions` type and extension.** A public tenant type whose member, `defaultInstructions`, holds `messages`: a list of system messages, each with `content`. The extension transforms requests with the existing port of upstream `defaultInstructionsMiddleware`: it prepends the messages, in order, only when the call has no system message, and never replaces one. It's off by default.
- **`default-settings` type and extension.** A public tenant type whose member, `defaultSettings`, allows only `temperature`, `topP`, `topK`, `maxOutputTokens`, `stopSequences`, `seed`, `presencePenalty` and `frequencyPenalty`. The extension fills only the fields the call leaves unset, using the existing `DefaultSettings` middleware. Headers, provider options, tools, tool choice, response format, reasoning and unknown keys are refused, by Grafana's own validation and by admission. It's off by default.
- **Scope and failure.** Both apply to operator-owned and customer-owned calls from every source, run only in the request transform phase, use failure policy `skip`, and register no collectors.
- **Registry, binding and manifest.** Both types join the public `tenanttype` registry, with descriptions and examples. Their implementations bind to them, and `DefaultExtensions()` returns both. The generator adds the `defaultInstructions` and `defaultSettings` members to the manifest.
- **Shipped behavior changes.** Grafana now accepts valid `Middleware` resources of these two types. The Gateway now reads each namespace's resources for calls, cached as before, which is the first real API traffic from #765's reader.
- **End-to-end evidence.** The checks deferred from the `Middleware` kind move into the CI suite against Grafana: a save without a folder, a nested round-trip, update and disable, delete, `403` for Viewers and Editors on stored resources, Grafana's own validation messages without sentinel values, and the runtime effect through the language-model route. That covers applied, then disabled, and then removed, within a bounded poll for the cache TTL. The suite also echoes the API-access record and keeps service-account tokens out of Playwright traces.
- **Docs.** A guide for org admins covers creating these resources, roles, defaults, how settings combine with call values, and propagation. Updates to the extension guide, architecture walkthrough, glossary and godoc.

## Capabilities

### New Capabilities

- `gateway-tenant-call-defaults`: the two shipped tenant extension types, their behavior, their scope and failure policy, their end-to-end evidence and the org admin guide.

### Modified Capabilities

- `gateway-extensions`: the shipped definitions and registry are no longer empty; the admission, evidence and read requirements move from an empty shipped build to the shipped types.

## Impact

- **Code:** `ai-gateway/pkg/extension/tenanttype` (two types), a new internal package for the implementations, `ai-gateway/pkg/gateway` (`DefaultExtensions`), generated `kinds/middleware_members.cue`, `src/app-sdk-manifest.json`, `pkg/internal/manifestdata`, `pkg/internal/middleware/v1alpha1` and `src/generated/middleware/`.
- **End-to-end suite:** `tests/middleware.spec.ts`, `tests/accounts.ts`, `tests/fake-anthropic.mjs` and `scripts/e2e.sh`.
- **APIs:** `gateway.DefaultExtensions()` returns two definitions, and `tenanttype` exports the two types. ProviderWire, the routes and the extension contract are unchanged.
- **Platform:** the shipped plugin lists `middlewares` in each namespace it serves calls for. A manual check on a development deployment confirms per-stack configuration and successful reads (see design.md).
- **Parity-sensitive:** both extensions port upstream `ai@7.0.159` middleware semantics. design.md compares `default-settings` with `defaultSettingsMiddleware` and classifies each difference. The tenant schema is a Gateway-owned restriction.
