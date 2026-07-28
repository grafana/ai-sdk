// Package grafana provides access to Grafana's hosted provider-wire endpoint
// for internally provisioned Grafana services. NewWithCloudAuth exchanges an
// internally provisioned Cloud Access Policy token; NewWithAccessToken forwards
// a short-lived access token supplied by an internal control plane. Public
// authentication for external Grafana Cloud users is not yet supported.
package grafana
