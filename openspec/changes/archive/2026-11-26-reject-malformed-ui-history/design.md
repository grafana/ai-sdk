## Context

At registration `0e8e0543` (`ai@7.0.173`), `ui/validate-ui-messages.ts` and `types/citation.ts` require fields before conversion. The Agent stream entry point calls that validation before invoking the model. In Go, some required strings in UI history and citations become zero values at decode; the citation source discriminator is not retained at all.

## Goals / Non-Goals

- Check presence before losing it, without changing Go's public value types or the Agent helper.
- Do not copy the upstream schema, add runtime JSON Schema validation, or change optional field normalization and configured tool schema gates.
- Do not implement steering. The pinned validator has a steering-message part with nested text/file parts, but Go has no represented steering part or its between-steps conversion. Preserve unknown parts for persistence and keep the Agent's existing rejection of unsupported parts.

## Decisions

- Use private UI JSON decode helpers and ordinary `encoding/json` struct matching for fields that collapse missing/null into a valid zero value. Reuse existing raw JSON and pointer representations where they preserve presence.
- Apply the citation requirements only within persisted text-part decoding, not provider-domain citations or SSE chunk decoding.
- Keep existing Agent meaning and schema checks. Use the registered TypeScript validator as the differential oracle for synthetic missing/null mutations, rather than maintaining a second validation schema.

## Risks / Trade-offs

- Persisted inputs that relied on silent repair now fail with a field error. UI encoding retains the required approval ID even when empty; existing Agent identity validation is unchanged. Documented Go key matching and represented round trips remain supported.
- This is required-presence parity for represented fields, not full upstream schema parity.
