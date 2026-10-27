# Spec Delta

## ADDED Requirements

### Requirement: Stored reasoning-model text references require preceding reasoning

Stored assistant text on reasoning models SHALL use an item reference only when reasoning precedes it in the same assistant message. Otherwise the text SHALL be reconstructed inline. Repeated eligible text item IDs within one assistant message SHALL produce one reference. Conversation skipping and non-reasoning stored-text behavior SHALL remain unchanged.

#### Scenario: Reasoning was removed from stored history
- **WHEN** stored reasoning-model text retains an item ID but no preceding reasoning part
- **THEN** unary and streaming request conversion SHALL send inline assistant text instead of an invalid stored reference
- **AND** shared Mantle conversion SHALL retain the same safeguard

#### Scenario: Repeated references share a reasoning item
- **WHEN** multiple text parts carry the same stored item ID after a reasoning part
- **THEN** one item reference SHALL be sent for that text ID
