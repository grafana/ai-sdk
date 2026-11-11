# Spec Delta

## ADDED Requirements

### Requirement: Catalog management documentation
Operator documentation SHALL explain managing catalogs from the app, which roles read, write and call catalogs, and when changes take effect: catalog edits apply on the next call, while removed roles and revoked tokens can take longer because of platform caches. The catalog package guide SHALL distinguish stack catalogs from the Go catalog package.

#### Scenario: Operator reads how changes take effect
- **WHEN** an operator reads the stored catalog documentation
- **THEN** they SHALL find the catalog page, the three roles, and that catalog edits apply on the next call while role and token removals can take longer

#### Scenario: Developer reads the catalog package guide
- **WHEN** a developer reads the catalog package guide
- **THEN** it SHALL say that stack catalogs are stored `ModelCatalog` resources the Gateway serves, and link to their documentation
