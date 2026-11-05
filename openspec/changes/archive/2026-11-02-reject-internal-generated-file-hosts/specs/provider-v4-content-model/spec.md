## ADDED Requirements

### Requirement: Generated-file downloads reject internal hostnames

Generated-file HTTP(S) downloads SHALL reject hostnames ending in `.internal`
after lowercasing and removing trailing dots. This check SHALL apply to the
initial URL and every redirect before DNS lookup or connection to that target.
Existing local-hostname and private-address restrictions SHALL remain in force.

#### Scenario: Initial internal hostname is rejected before network access

- **WHEN** a generated-file URL uses `host.internal` or `HOST.INTERNAL.`
- **THEN** the download SHALL return a hostname validation error without DNS lookup or dialing, even if the configured resolver would return a public address

#### Scenario: Redirect to an internal hostname is rejected

- **WHEN** a public generated-file URL redirects to a hostname ending in `.internal`, including uppercase or trailing-dot forms
- **THEN** the download SHALL reject the redirect before DNS lookup or dialing its target

#### Scenario: Similar public hostnames remain allowed

- **WHEN** a generated-file URL uses `internal.example.com`, `service.internal.example.com`, `service.notinternal`, or `.internal` only in its path
- **THEN** the hostname check SHALL allow it and retain all existing address validation
