# Proposal

## Why

A Grafana app plugin's background work, such as a scheduled job, has no user to call for. In `self-hosted` mode the Gateway serves its configured catalog only to users and service accounts, so such a plugin cannot use the operator's models when it calls as itself, and the operator has no setting to allow it. The internal service allowlist that allows a named service in `cloud` mode is refused at startup in `self-hosted` mode.

Self-hosted Grafana can now identify the plugin: [grafana/grafana#139015](https://github.com/grafana/grafana/pull/139015) signs `serviceIdentity` as `plugin:<plugin ID>` on the access tokens Grafana issues its plugins, and Grafana carries it into the actor chain it delegates to the Gateway. The allowlist therefore has a signed value to match in self-hosted Grafana too.

## What Changes

- Accept `access_internal_services` in both access modes, with the existing entry rules.
- Give a request whose direct caller's service identity is on the allowlist configured-catalog access in `self-hosted` mode, as in `cloud` mode, whether it calls alone or for a user, and attribute it to that service.
- Keep every other `self-hosted` decision: users and service accounts get the configured catalog, request-BYOK controls stay refused, and every other caller is refused.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-caller-access`: the allowlist applies in both modes, and allowlisted callers get configured access and service attribution in `self-hosted` mode.
- `gateway-openai-chat-completions-adapter`: configured Chat callers include allowlisted self-hosted services.

## Impact

- No change to `cloud` mode, BYOK, catalog routes, the first-actor rule, or entry validation. A `self-hosted` deployment without the setting behaves as before.
- Operators opt in per service. A self-hosted plugin's entry is `plugin:<plugin ID>` and needs a Grafana version that signs the plugin's service identity; with an older Grafana, the entry matches nothing.
- Update the setting's description and the self-hosted access docs.

## Non-goals

- No default allowlist, and no allowance for every plugin.
- No matching on actor subjects or unsigned request data.
