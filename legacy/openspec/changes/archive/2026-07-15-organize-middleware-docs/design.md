## Context

The fixed documentation taxonomy places middleware overview and integration
pages in `docs/guides/`. The directory now mixes application workflows with a
growing SDK extension point. Providers already demonstrate a scalable pattern:
a dedicated directory for an overview and one page per integration while the
main index remains organized by developer outcome.

## Goals / Non-Goals

**Goals:**

- Give middleware narrative documentation a stable top-level section.
- Use descriptive page names independent of Go package shorthand.
- Preserve outcome-oriented navigation in `docs/README.md`.
- Keep links, footers, contributor guidance, and active specifications aligned.

**Non-Goals:**

- Change middleware APIs, module paths, or runtime behavior.
- Create nested middleware categories before the page count requires them.
- Modify archived OpenSpec artifacts.
- Move package godoc from its source modules.

## Decisions

- Add the singular `docs/middleware/` directory to match the Go extension point
  and the existing `middleware/` source tree.
- Use `overview.md`, `structured-logging.md`, `prometheus.md`,
  `context-enrichment.md`, and `agent-observability.md` as descriptive filenames.
- Move the current middleware overview into the new section. Application
  workflows such as testing, retry, fallback, and streaming remain in
  `docs/guides/`.
- Add one middleware group to `docs/README.md`. Production navigation links to
  the relevant integrations without duplicating a separate package taxonomy.
- Update active specifications and contributor guidance. Archived specifications
  continue to record their original paths.

Alternatives considered:

- `docs/guides/middleware/` keeps middleware subordinate to generic guides and
  does not address the ownership concern.
- `docs/observability/` fits current telemetry integrations but excludes request
  enrichment and future policy or transformation middleware.
- `docs/integrations/` overlaps the established provider section and gives the
  directory no clear boundary.

## Risks / Trade-offs

- Existing deep links will change → update every repository-owned relative link
  in the same commit and rely on docs lint for validation.
- A new top-level section increases taxonomy size → keep the directory focused
  on middleware overview and middleware-specific integration guidance.
- The flat directory may eventually grow large → introduce subgroups only when
  concrete page volume warrants them.
