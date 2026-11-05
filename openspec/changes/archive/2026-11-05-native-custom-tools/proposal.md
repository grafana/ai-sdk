# Native custom tools

Extend compatible 3.0.82's function-only tool transport with custom tools using
the common provider-defined openai.custom tool representation already implemented
by Responses 4.0.106. Preserve raw text deltas through InputEncodingText and final
canonical JSON strings, with a consumed per-part history marker for continuations
without declarations. This is an intentional provider extension, not an upstream
baseline upgrade. Native grammar compilation remains provider-owned.
