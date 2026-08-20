## ADDED Requirements

### Requirement: Agent-discoverable release intent
The repository SHALL provide a concise local skill whose metadata triggers for
release intent, changelog, version, tag, and module publication tasks.

#### Scenario: Agent handles a release-relevant code change
- **WHEN** an agent changes public SDK behavior and the release skill is available
- **THEN** the skill directs the agent to choose the commit type that produces the intended release

#### Scenario: Agent handles non-release work
- **WHEN** an agent determines that a change does not affect a published module
- **THEN** the skill directs the agent to use a non-releasing commit type and state that decision without inventing a version bump

#### Scenario: Mixed bump levels across modules
- **WHEN** one body of work is a feature for one module and a fix for another
- **THEN** the skill directs the agent to split it into separate commits rather than applying one type to everything

### Requirement: Read-only agent release inspection
The agent skill SHALL delegate version calculation, changelog rendering, and
tagging to the release system, and SHALL restrict agents to read-only release
commands.

#### Scenario: Agent previews a release
- **WHEN** an agent is asked what would be released
- **THEN** it runs the read-only preview command and reports its output

#### Scenario: Agent validates release configuration
- **WHEN** an agent adds a published module or changes tag settings
- **THEN** it runs the release configuration check and fixes reported repository problems rather than bypassing them

### Requirement: Agents do not publish
The agent skill SHALL prohibit creating tags, creating GitHub Releases, and
merging the release pull request unless the user explicitly requests that action
in the current task.

#### Scenario: Ambiguous release request
- **WHEN** a user asks an agent to prepare a release or update changelogs
- **THEN** the agent does not tag, publish, or merge the release pull request

#### Scenario: Manual version edit
- **WHEN** an agent is tempted to edit the version manifest or a generated changelog section
- **THEN** the skill directs it to change the commit history or configuration instead
