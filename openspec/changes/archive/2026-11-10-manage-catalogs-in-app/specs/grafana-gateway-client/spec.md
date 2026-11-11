# Spec Delta

## ADDED Requirements

### Requirement: Grafana models and customer catalogs guidance
The client guide SHALL explain how an application or plugin offers Grafana's configured models and lets a customer opt in to their own stored catalog: one client per route, the customer's choice stored in the consuming plugin's settings, operator models in Cloud needing an allowlisted backend, catalog calls on behalf of the user, and why falling back from a customer model to Grafana models is discouraged.

#### Scenario: Plugin developer adds customer catalogs
- **WHEN** a plugin developer follows the guide to let customers choose their own catalog
- **THEN** the guide SHALL show one client for the Gateway URL and one for the catalog's base URL, and where to store the choice

#### Scenario: Customer model fails
- **WHEN** the guide describes a failing call to the customer's catalog
- **THEN** it SHALL recommend surfacing the error rather than retrying on Grafana models, and say why
