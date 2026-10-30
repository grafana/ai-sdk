# Proposal

## Why

OpenAI-compatible user messages cannot currently send uploaded file references, although the registered upstream adapter supports them. Issue #450 requires resolving the configured provider's exact reference entry before media conversion.

## What Changes

- Emit user references as `file.file_id`, including explicitly empty identifiers.
- Resolve the first, trimmed provider-name segment without normalizing reference keys or using the camel-cased metadata namespace.
- Reject missing or malformed references before transport; preserve mixed content order and existing metadata overlays.
- Add focused synthetic unary and streaming request tests and document the support boundary.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `provider-file-data-selection`: specify OpenAI-compatible uploaded user-file resolution and its request-boundary evidence.

## Impact

Only the OpenAI-compatible request converter, internal wire type, focused tests, provider guide and parity evidence metadata change. The public provider contract and frontend wire protocol remain unchanged.

## Non-goals

Multipart tool-result conversion remains owned by #362. This change does not upload files, fetch URLs, change provider option aliases or response metadata namespaces, or establish live server acceptance.
