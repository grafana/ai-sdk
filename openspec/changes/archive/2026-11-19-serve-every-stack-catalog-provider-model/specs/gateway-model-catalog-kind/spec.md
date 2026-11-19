# Spec Delta

## ADDED Requirements

### Requirement: Optional provider serving settings
A `ModelCatalog` provider SHALL accept optional `serveAllModels` (boolean) and `modelBlocklist` (array of backend model ID strings). An omitted or false switch SHALL mean listed models only, and an omitted or empty blocklist SHALL block no IDs. A blocklist SHALL be accepted and retained while the switch is off. The operator catalog schema SHALL remain unchanged.

#### Scenario: Existing catalog
- **WHEN** an existing catalog omits both fields
- **THEN** it SHALL remain valid and its providers SHALL serve listed models only

#### Scenario: Prepare a blocklist before enabling
- **WHEN** a writer saves valid blocked IDs with `serveAllModels` off
- **THEN** the write SHALL be accepted and a read SHALL return those IDs without enabling unlisted models

### Requirement: Validate provider blocklists
Each provider's `modelBlocklist` SHALL contain at most 128 distinct, exact, case-sensitive backend IDs. An ID SHALL be valid UTF-8, 1 to 2,048 bytes, with no control characters or leading or trailing whitespace. Admission SHALL reject invalid fields and a blocklist entry that equals a listed primary or fallback target's backend ID for that provider instance, regardless of the switch. Refusals SHALL identify the field without echoing the blocked ID or keys.

#### Scenario: Wrong field type
- **WHEN** a write supplies either setting with the wrong type
- **THEN** the catalog API SHALL refuse the write

#### Scenario: Invalid blocklist entries
- **WHEN** a blocklist has too many entries, a duplicate ID, an empty or whitespace-padded ID, a control character, or an ID exceeding the byte bound
- **THEN** admission SHALL refuse the write with `400`

#### Scenario: Conflicting primary or fallback
- **WHEN** a write would leave a blocked ID as a listed model's primary or fallback for the same provider instance
- **THEN** the write SHALL be refused with `400`, even if `serveAllModels` is off

#### Scenario: Same backend ID on another instance
- **WHEN** a listed target uses an ID blocked only by a different instance, even of the same provider type
- **THEN** the blocklist conflict check SHALL accept the write
