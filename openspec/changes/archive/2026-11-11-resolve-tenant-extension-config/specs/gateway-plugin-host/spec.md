# Spec Delta

## ADDED Requirements

### Requirement: Tenant configuration API access from the host
The app provider callback SHALL hand the API access configuration the host supplies to the Gateway's tenant configuration reader before any request is served. The reader SHALL use only that configuration and SHALL start no informer or watch. When the host supplies none, the Gateway SHALL treat the plugin as having no API access.

#### Scenario: Host supplies API access
- **WHEN** the plugin runtime builds the app with API access for the Gateway's group
- **THEN** the Gateway SHALL read `Middleware` resources through that configuration, as the plugin

#### Scenario: Host supplies no API access
- **WHEN** the plugin runtime builds the app without API access configuration
- **THEN** the Gateway SHALL make no reads and apply "Tenant configuration without API access"

#### Scenario: Idle Gateway
- **WHEN** a Gateway that started idle builds its app
- **THEN** it SHALL construct no reader

## MODIFIED Requirements

### Requirement: Kinds served by the plugin app
The plugin app SHALL manage each manifest kind with exactly the admission handlers the manifest declares, next to the route handler, and SHALL start no informers, watchers or reconcilers for them. The `Middleware` validator SHALL decide with the distribution's extension definitions and whether the plugin has API access. An idle Gateway SHALL refuse every admission review, so writes fail closed on a host that delegates no requests to it.

#### Scenario: App and manifest agree
- **WHEN** the app is built from the committed manifest
- **THEN** every declared kind SHALL be managed and each declared admission capability SHALL have its handler, with no undeclared handler

#### Scenario: Admission through the plugin protocol
- **WHEN** the host sends an admission review for a `ModelCatalog` with a delegated token
- **THEN** the plugin SHALL authenticate it like a route call and answer with the catalog validator's decision

#### Scenario: Middleware admission through the plugin protocol
- **WHEN** the host sends an admission review for a `Middleware` with a delegated token
- **THEN** the plugin SHALL authenticate it like a route call and answer with the decision for the extension definitions the distribution passed and the plugin's API access

#### Scenario: Idle Gateway
- **WHEN** an idle Gateway receives an admission review
- **THEN** it SHALL refuse it without validating the object
