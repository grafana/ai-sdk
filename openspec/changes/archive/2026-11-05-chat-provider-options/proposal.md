# Chat provider options and BYOK

Allow official Chat clients to use the shared Gateway account and routing policy through the providerOptions extension. Preserve native namespace fields through Chat translation and retain all post-translation guards.

The registered upstream reference is @ai-sdk/gateway 4.0.132, commit 028a025398782f4938303f48fbeee45c79af544b. Its LanguageModelV4 request preserves providerOptions. Exposing that common extension on Chat is an intentional Gateway adaptation, not an OpenAI standard field. Tests use synthetic native responses, not live-provider recordings.
