// Package grafana provides authenticated model discovery and inference for the
// Grafana AI Gateway plugin.
//
// NewWithServiceAccountToken sends a Grafana service account token as an
// Authorization bearer, and NewWithAccessToken forwards a caller-managed JWT.
// Both take the plugin's namespaced route prefix shown on the Gateway's
// overview page, for example
// https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-1.
//
// NewWithTokenExchange delegates internal token exchange and caching to
// Grafana authlib for an explicit audience. It takes the Gateway API root,
// https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1, and adds the
// namespace of each call, so one provider can serve several stacks:
// WithNamespace selects a call's stack, and WithOnBehalfOf exchanges a user ID
// token or delegated access token as its subject. Caller contexts control
// cancellation.
//
// Models support text generation and incremental text streaming through the
// provider.LanguageModel contract. The client has no dependency on the Gateway
// service module and never retries Gateway requests itself.
//
// GatewayOptions sends Gateway host controls under providerOptions.gateway:
// fallback models, provider order and filters, and request-supplied accounts.
// The Gateway's execution and routing attribution is returned unchanged under
// providerMetadata.gateway.
//
// The Gateway owns response and stream size limits, so the client has no
// configurable limits. Callers bound requests through their http.Client and the
// call context.
//
// SPDX-License-Identifier: Apache-2.0
package grafana
