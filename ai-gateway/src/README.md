# Grafana AI Gateway

The Grafana AI Gateway serves language-model calls through Grafana App Platform
routes. Clients use the [ProviderWire](https://github.com/grafana/ai-sdk/tree/main/ai-gateway/providerwire/v4)
protocol, which the Vercel AI SDK Gateway provider and the Grafana Go client
speak, against
`/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/<namespace>`.

The Gateway runs in one of two access modes:

- `cloud`: allowlisted internal services use the configured model catalog,
  directly or on behalf of a user. Other users and service accounts supply
  their own provider keys with each request.
- `self-hosted`: users and service accounts use the configured model catalog.

The plugin's overview page shows this stack's Gateway endpoint and a quick
start for Go and TypeScript. See the
[operator guide](https://github.com/grafana/ai-sdk/blob/main/ai-gateway/README.md)
for configuration, and the
[client guide](https://github.com/grafana/ai-sdk/blob/main/docs/guides/gateway-authentication.md)
for authentication.

Licensed under the
[GNU Affero General Public License v3.0](https://github.com/grafana/ai-sdk/blob/main/ai-gateway/LICENSE).
