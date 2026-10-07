## ADDED Requirements

### Requirement: Later user messages supersede unresolved approvals

ConvertToModelMessages SHALL omit approval-requested tool parts before the last user message without mutating UI history. It SHALL preserve approval requests after that boundary and preserve responded approvals regardless of the boundary.

#### Scenario: Abandoned approval before a new user turn

- **WHEN** an assistant has an unresolved approval request followed by a later user message
- **THEN** model history omits that tool call and approval request while retaining unrelated assistant content and the user turn

#### Scenario: Pending approval in the current turn

- **WHEN** an unresolved approval request appears after the last user message
- **THEN** model history retains that call and approval request
