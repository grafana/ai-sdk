# Preserve native Chat audio

Extend the opt-in Chat metadata codec with typed audio output and native audio
request/history types. The pinned @ai-sdk/openai-compatible 3.0.82 normalizer does
not preserve audio output; @ai-sdk/openai 4.0.106 Chat reads transcript only.
This deliberate Go extension retains Chat protocol identity and bytes for ingress
codecs without changing the common LanguageModel interface or forwarding raw output.
