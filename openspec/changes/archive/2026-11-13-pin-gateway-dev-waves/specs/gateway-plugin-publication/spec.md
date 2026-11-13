# Spec Delta

## MODIFIED Requirements

### Requirement: Dev version pin request
After a version is published to the dev catalog, the publish workflow SHALL open one auto-merge pull request in the deployment repository that pins each dev environment wave, `dev0` and `dev1`, to exactly that version, replacing any earlier pin pull request that has not merged. It SHALL request the pin only while the published commit is the head of `main` and the version ends in `+` and that commit's SHA. Pull requests, forks and failed or skipped publications SHALL NOT request a pin.

#### Scenario: Publication requests the dev pin
- **WHEN** commit `<sha>` is published to the dev catalog as `0.1.0-alpha.1+<sha>`
- **THEN** one pull request SHALL pin both the `dev0` and `dev1` waves to `0.1.0-alpha.1+<sha>`

#### Scenario: Newer publication replaces an open pin
- **WHEN** a pin pull request for an earlier version has not merged and a newer version is published
- **THEN** the pin pull request SHALL be replaced by one for the newer version

#### Scenario: Superseded commit requests no pin
- **WHEN** commit A's pin request is retried after a newer commit B was pushed to `main`
- **THEN** A's run SHALL succeed without requesting a pin
- **AND** the pin requested for B SHALL remain

#### Scenario: Head of main is unknown
- **WHEN** the head of `main` cannot be read when the pin is requested
- **THEN** the job SHALL fail without requesting a pin

#### Scenario: Failed publication requests no pin
- **WHEN** the dev publication for a commit fails or is skipped
- **THEN** no pin pull request SHALL be opened for that commit

#### Scenario: Version does not match the commit
- **WHEN** the published version is empty or does not end in `+<sha>` of the commit
- **THEN** the job SHALL fail without requesting a pin
