## ADDED Requirements

### Requirement: Chat applies tenant defaults before candidate translation

On both Chat routes, tenant defaults SHALL fill unset settings and
instructions after request mapping. Candidate translation SHALL then apply
Chat-specific defaults without changing the resolved settings and instructions.
Tenant defaults SHALL NOT change Chat request validation or set native provider
options.

#### Scenario: Explicit settings and instructions
- **WHEN** a valid Chat call supplies a system message, temperature zero, an
  accepted stop value or a supported token limit
- **THEN** the corresponding tenant default is suppressed, and each attempted
  candidate receives those caller values subject to native support.

#### Scenario: Absent or null settings and no instructions
- **WHEN** a valid Chat call omits a supported setting or uses its accepted
  null form, and the request has no system message
- **THEN** the enabled defaults supply configured values and prepend configured
  instructions before candidate translation.

#### Scenario: ProviderWire and Chat have different input contracts
- **WHEN** a Chat request sends an empty stop list
- **THEN** the adapter refuses it before defaults run; ProviderWire retains
  its existing explicit empty-list behavior.
