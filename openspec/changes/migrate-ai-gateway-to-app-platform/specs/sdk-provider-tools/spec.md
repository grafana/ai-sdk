# Spec Delta

## MODIFIED Requirements

### Requirement: Internal pins and standalone publication checks

Declared internal module pins SHALL resolve to revisions already merged into canonical main. Standalone builds SHALL be checked separately before module or plugin package publication.

#### Scenario: Publication uses merged pins and standalone checks
- **WHEN** a module or plugin package is prepared for publication
- **THEN** internal pins SHALL reference canonical-main merged revisions and standalone builds SHALL be checked separately.
