// Package grafana provides authenticated public model discovery for Grafana AI
// Gateway. Base URLs identify the ProviderWire API prefix, for example
// https://gateway.example/api/v1/aisdk. Cloud authentication delegates token
// exchange and caching to Grafana authlib; access-token authentication forwards
// a caller-managed token. Caller contexts control cancellation.
//
// Models support text generation and bounded incremental text streaming through
// the provider.LanguageModel contract. The client has no dependency on the
// Gateway service module and never retries Gateway requests itself.
//
// SPDX-License-Identifier: Apache-2.0
package grafana
