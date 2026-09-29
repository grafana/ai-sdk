// Package grafana provides authenticated public model discovery for Grafana AI
// Gateway. Base URLs identify the ProviderWire API prefix, for example
// https://gateway.example/api/v1/aisdk. NewWithCloudCredentials sends a
// stack-scoped Cloud Access Policy token through an authenticating proxy.
// NewWithTokenExchange delegates internal token exchange and caching to Grafana
// authlib; NewWithAccessToken forwards a caller-managed JWT. Caller contexts
// control cancellation.
//
// Models support text generation and bounded incremental text streaming through
// the provider.LanguageModel contract. The client has no dependency on the
// Gateway service module and never retries Gateway requests itself.
//
// SPDX-License-Identifier: Apache-2.0
package grafana
