## MODIFIED Requirements

#

## ADDED Requirements

#

#

### Requirement: Transport-safe model selectors
Model construction SHALL accept nonempty valid UTF-8 selectors up to 2,048 bytes
without whitespace or control characters, independently of configured-catalog
grammar. The server SHALL remain responsible for selector capability validation.

#### Scenario: Native suffix exceeds catalog grammar
- **WHEN** a valid selector contains native punctuation or exceeds the former 128-byte catalog limit
- **THEN** the client SHALL preserve it without discovery or rewriting

#### Scenario: Invalid transport selector
- **WHEN** a selector is empty, invalid UTF-8, over 2,048 bytes or contains whitespace/control characters
- **THEN** model construction SHALL fail before I/O
