## MODIFIED Requirements

### Requirement: Shared access and error boundaries

Namespace Chat SHALL preserve the host's configured/BYOK/refused decisions.
It SHALL use the operator catalog for configured callers and expose no BYOK
controls. Catalog Chat SHALL use the catalog language-model route's role,
parent check and access decision and use only the named catalog and its
decrypted keys. Plugin host errors SHALL use Chat envelopes; pre-plugin
failures MAY use the platform format.

#### Scenario: Configured callers
- **WHEN** a self-hosted user/service account or an allowlisted internal
  caller in either mode makes a valid Chat request
- **THEN** the shared selector uses configured catalog models.

#### Scenario: BYOK-only caller cannot obtain the catalog
- **WHEN** a cloud user/service account classified as BYOK sends a namespace Chat request
  without request-scoped provider controls
- **THEN** the shared selector returns invalid BYOK mapped to HTTP 400 and
  makes no native provider request.

#### Scenario: Host refuses account access
- **WHEN** host policy refuses the caller before reading its body
- **THEN** Chat's host-error writer returns its fixed permission document.

#### Scenario: Platform rejects before the plugin
- **WHEN** platform authentication or transport limits reject a request
- **THEN** the response need not use a Chat error envelope.

#### Scenario: Catalog and operator catalog share a model alias
- **WHEN** a caller permitted to invoke the named catalog requests the alias
  on the catalog Chat route
- **THEN** only the named catalog's candidates and keys are used, regardless
  of Gateway access mode.

#### Scenario: Plugin receives a missing or mismatched parent identity
- **WHEN** the platform-supplied route parent is missing or does not identify
  the requested catalog
- **THEN** the plugin returns the fixed Chat 500 document before reading
  the request body.

#### Scenario: Invalid catalog or disallowed native endpoint
- **WHEN** a stored catalog fails validation, has a missing decrypted key,
  or specifies an endpoint disallowed by stack-catalog policy
- **THEN** Chat returns the fixed 424 catalog-configuration outcome, with no
  provider I/O or catalog details.
