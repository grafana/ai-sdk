# Retain Responses refusal without changing defaults

Pinned openai 4.0.106 handles output_text but drops refusal. Chat compatibility
needs native refusal to avoid empty output or incorrect JSON validation errors.
Add a local opt-in, typed metadata and custom stream deltas, with no new V4
discriminator and no conversion into ordinary text.
