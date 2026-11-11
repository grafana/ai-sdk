## MODIFIED Requirements

### Requirement: Internal pins and standalone publication checks

Declared first-party requirements SHALL equal the version manifest and SHALL resolve to same-revision source through local replacements. Libraries SHALL publish only through a validated library release pull request, and each published version SHALL pass post-publication proxy verification.

#### Scenario: Publication uses merged pins and standalone checks
- **WHEN** a module or plugin package is prepared for publication
- **THEN** its first-party requirements SHALL equal the manifest versions being released or already tagged, and published library versions SHALL be verified as external consumers after tagging
