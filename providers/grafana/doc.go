// Package grafana provides authenticated model discovery and inference for the
// Grafana AI Gateway plugin. Base URLs are the plugin's namespaced route
// prefix, for example
// https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-1.
// NewWithServiceAccountToken sends a Grafana service account token as an
// Authorization bearer. NewWithTokenExchange delegates internal token exchange
// and caching to Grafana authlib for an explicit audience, and WithOnBehalfOf
// exchanges a user ID token or delegated access token as its subject.
// NewWithAccessToken forwards a caller-managed JWT. Caller contexts control
// cancellation.
//
// Models support text generation and incremental text streaming through the
// provider.LanguageModel contract. The client has no dependency on the Gateway
// service module and never retries Gateway requests itself.
//
// The Gateway owns response and stream size limits, so the client has no
// configurable limits. Callers bound requests through their http.Client and the
// call context.
//
// SPDX-License-Identifier: Apache-2.0
package grafana
