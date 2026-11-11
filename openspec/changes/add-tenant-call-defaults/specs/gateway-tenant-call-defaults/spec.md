# Spec Delta

## Purpose

Lets org admins give every model call in their stack default system instructions and default sampling settings through `Middleware` resources, with upstream middleware semantics and a Gateway-owned restriction on which settings tenants may set.

## ADDED Requirements

### Requirement: Default instructions type
The public tenant type `default-instructions` SHALL be off by default. Its member `defaultInstructions` SHALL hold a required, non-empty `messages` list, each item with a required, non-empty `content` string. No other key SHALL be accepted at any level.

#### Scenario: Valid configuration
- **WHEN** an Admin stores `defaultInstructions: {messages: [{content: "Answer concisely."}, {content: "Cite sources."}]}`
- **THEN** the write SHALL be accepted and read back unchanged

#### Scenario: Empty list or content
- **WHEN** the member's `messages` is empty, or an item's `content` is an empty string
- **THEN** admission SHALL refuse the write naming the member and the failing location

#### Scenario: Undeclared key
- **WHEN** a message item or the member carries another key, such as `providerOptions` or `role`
- **THEN** the write SHALL be refused without echoing the key's value

### Requirement: Default instructions applied
When enabled for a namespace, `default-instructions` SHALL prepend its messages, in order, as system messages to each call whose prompt has no system message, matching upstream `defaultInstructionsMiddleware` at the registered baseline. A call with a system message anywhere in its prompt SHALL reach the model with its prompt unchanged.

#### Scenario: Call without a system message
- **WHEN** a namespace enables two default messages and a unary or streaming call has only user and assistant messages
- **THEN** the model SHALL receive the two system messages, in order, followed by the call's messages

#### Scenario: Call with a system message
- **WHEN** the call's prompt has a system message, first or after other messages
- **THEN** the model SHALL receive the call's prompt unchanged, without default messages

### Requirement: Default settings type
The public tenant type `default-settings` SHALL be off by default. Its member `defaultSettings` SHALL set at least one of, and nothing beyond: numbers `temperature`, `topP`, `presencePenalty` and `frequencyPenalty`; integers `topK`, `maxOutputTokens` (at least 1) and `seed`, each within the signed 64-bit range; and `stopSequences`, a list of strings.

#### Scenario: Refused call controls
- **WHEN** the member sets `headers`, `providerOptions`, `tools`, `toolChoice`, `responseFormat`, `reasoning` or any other undeclared key
- **THEN** the write SHALL be refused and nothing SHALL be stored

#### Scenario: Invalid values
- **WHEN** the member sets `maxOutputTokens: 0`, a fraction for an integer field, an integer outside the signed 64-bit range, or no field at all
- **THEN** the write SHALL be refused naming the member and, for a field, its location

### Requirement: Default settings applied
When enabled, `default-settings` SHALL set each configured field only when the call leaves it unset, so the call's own value, including `0` and an empty stop sequence list, takes precedence. It SHALL NOT change the prompt, headers, provider options, tools, tool choice, response format or reasoning.

#### Scenario: Unset field
- **WHEN** a namespace configures `temperature: 0.2` and `maxOutputTokens: 1024`, and a call sets neither
- **THEN** the model SHALL receive temperature `0.2` and maximum output tokens `1024`

#### Scenario: Call's own values
- **WHEN** the call sets `temperature: 0` and `stopSequences: []`, and the namespace configures both
- **THEN** the model SHALL receive temperature `0` and no stop sequences

#### Scenario: Other parameters unchanged
- **WHEN** a call with headers, provider options and tools runs with default settings configured
- **THEN** the model SHALL receive those headers, provider options and tools exactly as the call sent them

### Requirement: Tenant call defaults scope
Both extensions SHALL be tenant-scope, apply to calls with operator-owned and customer-owned keys from every source, act only as request transforms, and use failure policy `skip`. The shipped Gateway SHALL register both, bound to their public types.

#### Scenario: Both key ownerships
- **WHEN** a namespace enables both extensions and runs a catalog call and a request BYOK call
- **THEN** both calls SHALL reach the model with the default instructions and settings applied

#### Scenario: Configuration that fails at read time
- **WHEN** a stored member no longer passes its type's schema when read
- **THEN** calls in that namespace SHALL continue without that extension, and the failure SHALL be counted

### Requirement: Tenant call defaults end-to-end evidence
The plugin end-to-end suite SHALL prove against the pinned Grafana, with the built plugin and shipped types: a save without a folder; a nested member read back unchanged; valid, refused and disable-only updates; a delete; Viewer and Editor reads with `403` on update and delete; Grafana's refusals without sentinel values; and the runtime effect.

#### Scenario: Runtime effect
- **WHEN** an Admin enables both types in a namespace, then disables one and deletes the other
- **THEN** a language-model call SHALL reach the provider with the instructions and settings applied within a bounded wait, and later calls SHALL reach it without them within a bounded wait

#### Scenario: Grafana's own refusals
- **WHEN** creates carry sentinel values in members with a wrong JSON type, a missing required field or an undeclared key
- **THEN** Grafana SHALL refuse them before admission, and no response body or plugin log SHALL contain a sentinel

### Requirement: Org admin defaults guide
The docs SHALL include a guide for org admins on these resources: creating, updating, disabling and deleting them, the roles that allow it, each type's fields and default state, how defaults combine with a call's own values and with provider limits, and how long a change takes to apply.

#### Scenario: Org admin sets stack defaults
- **WHEN** an org admin reads the guide
- **THEN** they SHALL find a complete resource for each type, that a client system message suppresses default instructions, that a call's own settings win, and that a change applies within about a minute per replica
