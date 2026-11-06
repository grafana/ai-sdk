# Chat message variants

## Why
Official clients send developer instructions, participant names, refusal history and text-array tool results. The initial Chat subset rejected them.

## What Changes
Map developer instructions to common system messages, retain refusal text for cross-provider history, and concatenate multipart tool text. For compatible candidates retain names and refusal metadata and use the native developer instruction role. Keep shared selection, translation and validation.

## Impact
Chat request conversion and a narrow native-options guard refinement: only an existing system instruction may select the developer instruction role. Other role replacements and content/tool overrides remain refused. The pinned OpenAI-compatible converter spreads native metadata; this is a Gateway admission adaptation, not an upstream baseline change.
