# Strict Chat request capabilities

Chat clients cannot assume provider-specific warning extensions are inspected.
Opt into rejection at the native request builder, before I/O, without duplicating
model capability tables or changing default V4 behavior.
