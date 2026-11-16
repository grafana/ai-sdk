# Preserve partial Chat completions

## Why
Valid empty completions and tool calls interrupted by token limits were converted into Gateway errors. Clients lost the native finish reason and partial arguments.

## What Changes
Accept empty stop results. Validate complete tool argument JSON and strict schemas at the final finish rather than while fragments arrive. Preserve length/content-filter arguments without claiming completion. Keep tool identity, declaration, choice and parallelism checks.

## Impact
Chat response projection only. The shared runtime retains lifecycle validation. The official OpenAI client exposes partial strings and finish reasons; the pinned Vercel provider also exposes native length/content-filter reasons. No fixture inputs or provider behavior change.
