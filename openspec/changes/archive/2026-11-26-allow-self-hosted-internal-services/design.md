# Design

## Context

`auth.ParseAccessPolicy` rejects a non-empty `access_internal_services` unless the mode is `cloud`, and `AccessPolicy.Decide` only checks the allowlist in `cloud` mode. The direct caller's service identity comes from the first actor's signed `serviceIdentity`, read by the platform authenticator in either mode.

When the Gateway moved to App Platform, the allowlist was specified as valid only in `cloud` mode without a stated reason. At the time, self-hosted Grafana signed no service identity on the tokens it issues its plugins, so a self-hosted allowlist could match nothing. [grafana/grafana#139015](https://github.com/grafana/grafana/pull/139015) changes that: Grafana signs `serviceIdentity: plugin:<plugin ID>` on its local plugin access tokens, and the token it delegates to the Gateway carries that value in the first actor.

Caller access is Grafana-specific (the **AI Gateway / Go extension** surface in `test/conformance/PARITY.md`). The registered Vercel baseline, `vercel/ai@9b8c25e582bb2ca97e459ba3f8f6b325cb9dfb22` (`@ai-sdk/gateway@4.0.152`), has no counterpart, so no client contract or coverage map entry changes.

## Design decisions

### One allowlist rule for both modes

Check the allowlist first in both modes, with the same exact, case-sensitive match on the first actor's signed service identity. The decision becomes:

| Mode | Caller | Access |
|---|---|---|
| either | Direct caller on the allowlist | configured, attributed to the service |
| `cloud` | Otherwise, user or service-account identity actor | BYOK |
| `self-hosted` | Otherwise, user or service-account identity actor | configured; BYOK controls refused |
| either | Anything else | refused with `HostErrorPermission` |

The first-actor rule is unchanged, so a service that relays an allowlisted service's token still does not gain configured access. BYOK stays `cloud`-only.

### Attribution follows the allowlist, not the mode

An allowlisted caller's observations carry its service identity in both modes, and extensions see the `internal-service` caller type. This changes one existing `self-hosted` outcome: an allowlisted service calling for a user was already served from the catalog as that user, and is now also attributed to the service. That matches `cloud` mode, and a deployment that does not list the service is unaffected.

### Opt-in only

The allowlist stays empty by default. Allowing every plugin was rejected: in `self-hosted` mode, the operator's roles decide which users may use the app, and a plugin calling as itself has no role, so each plugin that may spend the operator's keys without a user is named explicitly.

## Open questions

| Question | Recommendation |
| --- | --- |
| Should `self-hosted` entries be written as bare plugin IDs and prefixed by the Gateway? | No. Match the signed value exactly, as in `cloud` mode, and document the `plugin:<plugin ID>` form. |
