# Tasks

## 1. Public contract

- [x] 1.1 Remove `Definition.Sources` and `extension.Sources`, and document `Call.Source` and `Source` as an informational, growing set; verify `go build ./...` and `TestRun_CompilesFromAnExternalModule`.

## 2. Host

- [x] 2.1 Remove the sources registration checks and key the host's ordered lists by key ownership; verify `TestValidateDefinitions`, `TestApplies` and `TestHost_Compose`, including a source the vocabulary does not name yet.

## 3. Documentation

- [x] 3.1 Update the extensions guide and the glossary; verify `mise run lint-docs`.

## 4. Integration checks

- [ ] 4.1 Run the full check set and `openspec validate simplify-extension-applicability --strict`; verify all pass.

## Workflow follow-up

- Archive the change as the last commit of the branch.
