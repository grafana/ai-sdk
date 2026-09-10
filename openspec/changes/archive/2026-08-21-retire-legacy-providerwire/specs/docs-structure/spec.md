## MODIFIED Requirements

### Requirement: User-facing narrative is centralized in docs/

User-facing narrative documentation SHALL be centralized under `docs/` rather
than scattered across package-local READMEs. Module/package `README.md` files
that document user-facing setup, usage, or behavior SHALL have their content
folded into the appropriate `docs/` page and SHALL be removed. All `doc.go`
files SHALL remain co-located with their code as the godoc API reference and
SHALL NOT be moved. Contributor/tooling READMEs that are not user-facing (e.g.
under `test/`) MAY remain co-located. Documentation for a removed package SHALL
be deleted or replaced by an indexed retirement note rather than retained as
current usage guidance.

#### Scenario: User-facing package READMEs are centralized

- **WHEN** a retained package has a README documenting user-facing setup or behavior
- **THEN** its content SHALL live in the relevant `docs/` page and the package-local README SHALL be removed

#### Scenario: doc.go files are not moved

- **WHEN** documentation centralization is applied to retained packages
- **THEN** their `doc.go` files remain in their original locations as the godoc API reference

#### Scenario: Removed package documentation is not current guidance

- **WHEN** a package or module is removed from the repository
- **THEN** its former provider or guide page SHALL NOT remain indexed as an available capability
- **AND** any indexed retirement note SHALL distinguish historical source from a currently installable API
