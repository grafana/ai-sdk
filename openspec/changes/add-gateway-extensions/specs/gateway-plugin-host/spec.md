# Spec Delta

## MODIFIED Requirements

### Requirement: Host-owned lifecycle and bounded final flush
Draining in-flight requests SHALL belong to the host; the Gateway SHALL NOT depend on receiving a termination signal and SHALL NOT cancel requests on its own when shutdown begins. After the plugin protocol server stops, the Gateway SHALL flush Agent Observability records, attempt records and extension observations, and close extension instances, concurrently under one shared deadline of 1 second, then exit. Records emitted after the flush starts SHALL be dropped and counted, not block or fail.

#### Scenario: Host stops the plugin
- **WHEN** the host stops the plugin process through the plugin protocol, with host tracing unconfigured or its shutdown completing promptly
- **THEN** the Gateway SHALL run its final flush without a termination signal and exit before the host's forced-kill delay

#### Scenario: Final flush exceeds budget
- **WHEN** queued telemetry cannot be delivered within the final-flush budget
- **THEN** the Gateway SHALL abandon it, account for the loss with bounded counters or fixed diagnostics, and exit before the budget ends

#### Scenario: Deployment guidance matches request bounds
- **WHEN** operator documentation describes deployment
- **THEN** it SHALL state that the host's termination grace period must exceed the platform request deadline plus drain time, and that host tracer shutdown shares the final-flush window

#### Scenario: Extension close shares the budget
- **WHEN** an extension instance's close blocks, fails or panics during the final flush
- **THEN** the Gateway SHALL stop waiting for it at the shared deadline, count the failure, and still flush the other sinks and exit before the budget ends

### Requirement: Public composition entrypoint
A public Go package in the Gateway module SHALL expose one entrypoint that runs the plugin backend from an options value. The entrypoint SHALL own the plugin runtime, the plugin ID, the platform authenticator, the process logger, environment and metrics registerer, the app provider callback that receives the host's API access configuration, the lifecycle events and the final flush. Options SHALL NOT override any of them.

#### Scenario: Shipped main is a thin caller
- **WHEN** the shipped plugin backend starts
- **THEN** its `main` SHALL only call the entrypoint with the shipped extension definitions and no mandatory extension, and exit non-zero when the entrypoint reports failure
- **AND** startup, idle, health, lifecycle events, final flush and exit behavior SHALL be unchanged

#### Scenario: Distribution compiled from another module
- **WHEN** a Go module outside the Gateway module imports the package and calls the entrypoint with zero options, or with an extension it defines
- **THEN** it SHALL compile without importing any Gateway internal package

#### Scenario: Distribution keeps the plugin identity
- **WHEN** any distribution runs the entrypoint
- **THEN** the plugin SHALL identify as the plugin ID declared in the plugin metadata and verify delegated tokens with the same authenticator as the shipped backend

#### Scenario: No implicit registration
- **WHEN** a distribution imports the package
- **THEN** importing SHALL register nothing; behavior SHALL depend only on the options passed to the entrypoint and the host environment

## ADDED Requirements

### Requirement: Extension options on the entrypoint
The entrypoint options SHALL select only the distribution's extension definitions and which of them are mandatory. Definitions and the mandatory list SHALL be validated before the Gateway reads any setting, including when it would start idle, so an invalid build fails startup on every host.

#### Scenario: Invalid definitions on an idle host
- **WHEN** a distribution passes two definitions with the same name and the host supplies no platform signing keys
- **THEN** startup SHALL fail naming the duplicate instead of starting idle

#### Scenario: Idle host with valid definitions
- **WHEN** a distribution passes valid definitions and the host supplies no platform signing keys
- **THEN** the Gateway SHALL start idle without reading the extension setting or constructing any extension

### Requirement: Operator extension setting
The Gateway SHALL read operator extension configuration from the `extensions` setting (`GF_PLUGIN_EXTENSIONS`): one strict YAML mapping from extension name to an entry with optional `enabled`, optional `config` and one `<secret>Env` key per declared secret. An unset or blank setting SHALL mean no entries. The setting SHALL be at most 64 KiB, and aliases, merge keys, unknown keys and trailing documents SHALL be refused.

#### Scenario: Entry with a secret
- **WHEN** the setting is `quota: {config: {limit: 100}, apiKeyEnv: GF_PLUGIN_QUOTA_KEY}` and `quota` declares secret `apiKey`
- **THEN** the `quota` factory SHALL receive the configuration `{"limit": 100}` and the secret `apiKey` read from `GF_PLUGIN_QUOTA_KEY`

#### Scenario: Unknown extension or key
- **WHEN** the setting names an extension the distribution does not register, or an entry has an undeclared key
- **THEN** startup SHALL fail naming the setting, the extension and the key, without their values

#### Scenario: Oversized or malformed setting
- **WHEN** the setting is larger than 64 KiB, is not a mapping, or uses an alias
- **THEN** startup SHALL fail with a reason that names the setting without its value

### Requirement: Extension secret handling
Extension secrets SHALL be read from the environment only after every setting, the model catalog and the extension configuration are validated. An unset or empty secret variable of an enabled extension SHALL fail startup naming the variable, not its value. Resolved secret values SHALL be redacted from Gateway-produced failure summaries like configured provider keys.

#### Scenario: Missing secret
- **WHEN** an enabled extension's entry names `apiKeyEnv: GF_PLUGIN_QUOTA_KEY` and the variable is unset
- **THEN** startup SHALL fail naming `GF_PLUGIN_QUOTA_KEY`

#### Scenario: Provider echoes an extension secret
- **WHEN** a provider failure message contains a resolved extension secret
- **THEN** the published failure summary SHALL redact it

#### Scenario: Disabled extension
- **WHEN** an entry sets `enabled: false` and names an unset secret variable
- **THEN** startup SHALL succeed without reading that variable
