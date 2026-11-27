# Tasks

## 1. Access policy

- [x] 1.1 Accept the allowlist in both modes in `ParseAccessPolicy`, keeping the entry rules. Replace the self-hosted startup failure tests in `config` and `gateway` with acceptance and invalid-entry cases.
- [x] 1.2 Check the allowlist in both modes in `Decide`. Add `self-hosted` cases for an allowlisted service alone and for a user, a service off the allowlist, a user whose direct caller is off the allowlist, a relay, a case near-match and another namespace.

## 2. Docs

- [x] 2.1 Update the setting and **Choose who can use it** in `ai-gateway/README.md`, the App Platform app section of the authentication guide, the Chat Completions guide, and the architecture walkthrough. Check the glossary for drift.

## 3. Validation

- [x] 3.1 Run the Gateway Go tests, docs lint and strict OpenSpec validation. Sync the deltas and archive the change.
