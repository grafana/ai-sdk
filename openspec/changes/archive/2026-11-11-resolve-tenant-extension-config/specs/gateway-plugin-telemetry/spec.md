# Spec Delta

## ADDED Requirements

### Requirement: Tenant configuration read telemetry
The Gateway SHALL count tenant configuration reads by a fixed result, `success` or `failed`, and enabled resources of types the build does not implement, with no namespace, type, caller or error label. Each failed read SHALL emit one warning with the namespace and a fixed class: timeout, refused, not found, unexpected status, invalid response or error. It SHALL NOT carry a response body, error text or configuration value.

#### Scenario: Refused read
- **WHEN** the platform refuses a namespace's list with `403` and a body containing a sentinel
- **THEN** the read counter SHALL increase under `failed`, and the warning SHALL carry the namespace and class `refused`, and no part of the body

#### Scenario: Type not implemented
- **WHEN** a namespace stores an enabled resource of a type the build does not implement
- **THEN** the counter SHALL increase once, with no label naming the namespace or type

### Requirement: Plugin API access log
When a running Gateway builds its app, it SHALL emit one informational log record stating whether the host gave the plugin API access, whatever extensions the build registers. The record SHALL carry no URL, token or other credential.

#### Scenario: Host with API access
- **WHEN** the end-to-end suite starts Grafana with the built plugin
- **THEN** the plugin log SHALL contain one record stating that the plugin has API access, and no credential

### Requirement: Tenant configuration failure events
A stored configuration that fails its schema, and a factory that fails or panics for a namespace, SHALL be counted as an extension event with the extension's name, phase `configure` and event `failed`. The Gateway SHALL log one warning with the extension, namespace, failure policy and a fixed class, and no configuration value or error text.

#### Scenario: Invalid stored configuration
- **WHEN** a namespace's stored configuration for an extension fails its schema, and the stored values contain a sentinel
- **THEN** the extension event counter SHALL increase under phase `configure` and event `failed`, and no log record SHALL contain the sentinel
