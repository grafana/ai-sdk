## Why

Anthropic schema conversion sorts caller property names, changing the order in which a model generates dependent fields (#409). Native output, JSON-tool fallback and function tools must retain schema member order.

## What Changes

- Preserve schema ordering while retaining existing native sanitization.
- Pass unsanitized tool schemas as raw JSON, retaining numeric tokens.
- Verify unary and streaming HTTP request bodies without a live model call.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `anthropic-structured-output`: preserve schema member order on native and tool request paths.

## Impact

Only Anthropic request conversion changes. No public API, dependencies, provider responses or baseline pins change.
