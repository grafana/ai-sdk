# Preserve the Chat token-limit field

Pinned compatible 3.0.82 always emits max_tokens. Reasoning Chat endpoints may
require max_completion_tokens. Add a typed local selector that changes only the
field name for the common budget; defaults remain pinned. Chat derives the
selector from its public input and shared guards prevent alternate raw budgets.
