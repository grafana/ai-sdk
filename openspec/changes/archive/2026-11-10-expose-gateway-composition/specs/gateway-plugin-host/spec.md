# Spec Delta

## ADDED Requirements

### Requirement: Public composition entrypoint
A public Go package in the Gateway module SHALL expose one entrypoint that runs the plugin backend from an options value. The entrypoint SHALL own the plugin runtime, the plugin ID, the platform authenticator, the process logger, environment and metrics registerer, the app provider callback that receives the host's API access configuration, the lifecycle events and the final flush. Options SHALL NOT override any of them.

#### Scenario: Shipped main is a thin caller
- **WHEN** the shipped plugin backend starts
- **THEN** its `main` SHALL only call the entrypoint with zero options and exit non-zero when the entrypoint reports failure
- **AND** startup, idle, health, lifecycle events, final flush and exit behavior SHALL be unchanged

#### Scenario: Distribution compiled from another module
- **WHEN** a Go module outside the Gateway module imports the package and calls the entrypoint with zero options
- **THEN** it SHALL compile without importing any Gateway internal package

#### Scenario: Distribution keeps the plugin identity
- **WHEN** any distribution runs the entrypoint
- **THEN** the plugin SHALL identify as the plugin ID declared in the plugin metadata and verify delegated tokens with the same authenticator as the shipped backend

#### Scenario: No implicit registration
- **WHEN** a distribution imports the package
- **THEN** importing SHALL register nothing; behavior SHALL depend only on the options passed to the entrypoint and the host environment

### Requirement: Experimental composition API
The public composition package SHALL be documented as v0 and experimental: its API MAY change in any minor release. Documentation SHALL NOT present it as a stable extension contract.

#### Scenario: Package documentation states stability
- **WHEN** a developer reads the package documentation
- **THEN** it SHALL state that the API is v0, experimental and may break in minor releases
