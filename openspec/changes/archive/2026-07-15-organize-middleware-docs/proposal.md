## Why

Middleware documentation currently shares the generic `docs/guides/` directory
with application workflows. A dedicated section gives existing and future
middleware integrations a stable, discoverable home as the module set grows.

## What Changes

- Add `docs/middleware/` as a first-class narrative documentation section.
- Move the middleware overview and integration pages into that section with
  descriptive filenames.
- Group middleware links in the documentation index while keeping navigation
  organized by developer outcome.
- Update cross-links, page footers, contributor guidance, and the active
  documentation structure contract.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `docs-structure`: Add the middleware section to the required documentation
  taxonomy and define it as the home for middleware overview and integration
  guidance.

## Impact

The change affects Markdown paths and links under `docs/`, documentation
contributor guidance, and the active `docs-structure` OpenSpec. It does not
change Go APIs, package paths, runtime behavior, or archived OpenSpec history.
