# Agent Release Workflow

## Purpose

Define how coding agents discover and safely operate the repository's
command-backed release workflow, including release-intent decisions, selective
module preparation, and explicit publication authorization.

## Requirements

### Requirement: Agent-discoverable release workflow

The repository SHALL provide a concise local skill whose metadata triggers for
release intent, changelog, version, tag, and module publication tasks.

#### Scenario: Agent handles a release-relevant code change

- **WHEN** an agent changes public SDK behavior and the release skill is available
- **THEN** the skill directs the agent to create or update explicit release intent and validate the resulting plan

#### Scenario: Agent handles non-release work

- **WHEN** an agent determines that a change does not affect a public release
- **THEN** the skill directs the agent to state that decision without inventing a version bump

### Requirement: Command-backed agent decisions

The agent skill SHALL delegate fragment creation, validation, planning,
preparation, and publication checks to the repository release command rather
than asking the agent to calculate versions or edit changelogs manually.

#### Scenario: Agent previews a release

- **WHEN** an agent is asked what would be released
- **THEN** it runs the read-only plan command and reports its output

#### Scenario: Agent prepares a release

- **WHEN** a user explicitly asks an agent to prepare a release
- **THEN** it runs the preparation command, reviews the resulting diff, and does not publish tags

#### Scenario: Agent prepares named modules

- **WHEN** a user asks an agent to release only specific modules
- **THEN** it passes explicit module selectors to both planning and preparation and reports the intent left pending

### Requirement: Explicit publication authorization

The agent skill SHALL prohibit confirmed publication unless the user explicitly
requests external release publication in the current task.

#### Scenario: Ambiguous release request

- **WHEN** a user asks to update release files without explicitly asking to publish
- **THEN** the agent may prepare and validate but does not pass the publication confirmation flag

#### Scenario: Explicit publication request

- **WHEN** a user explicitly asks to publish the prepared release
- **THEN** the agent validates the exact plan and may invoke confirmed publication after reporting the external effects
