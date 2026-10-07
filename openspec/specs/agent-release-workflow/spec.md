# Agent Release Workflow

## Purpose

Define how repository guidance directs coding agents to express release intent
for the pull-request-title-derived release system and avoid publication.

## Requirements

### Requirement: Documented release intent

The repository SHALL direct agents to the release intent rules in
`CONTRIBUTING.md` from `AGENTS.md`.

#### Scenario: Agent handles a release-relevant code change

- **WHEN** an agent changes public SDK behavior
- **THEN** the repository guidance directs the agent to choose the pull request title type that produces the intended release

#### Scenario: Agent handles non-release work

- **WHEN** an agent determines that a change does not affect a published module
- **THEN** the repository guidance directs the agent to use a non-releasing title type without inventing a version bump

#### Scenario: Mixed bump levels across modules

- **WHEN** one body of work is a feature for one module and a fix for another
- **THEN** the repository guidance directs the agent to split the work into separate pull requests rather than applying one type to everything

### Requirement: Read-only agent release inspection

The repository guidance SHALL delegate version calculation, changelog
rendering, and tagging to the release system, and SHALL identify read-only
release inspection commands.

#### Scenario: Agent previews a release

- **WHEN** an agent is asked what would be released
- **THEN** it runs the read-only preview command and reports its output

#### Scenario: Agent validates release configuration

- **WHEN** an agent adds a published module or changes tag settings
- **THEN** it runs the release configuration check and fixes reported repository problems rather than bypassing them

### Requirement: Agents do not publish

The repository guidance SHALL keep release preparation separate from
publication and require an explicit request before creating tags, creating
GitHub Releases, or merging release pull requests.

#### Scenario: Ambiguous release request

- **WHEN** a user asks an agent to prepare a release or update changelogs
- **THEN** the agent does not tag, publish, or merge the release pull request

#### Scenario: Manual version edit

- **WHEN** an agent is tempted to edit the version manifest or a generated changelog section
- **THEN** the repository guidance directs it to change the pull request title or configuration instead
