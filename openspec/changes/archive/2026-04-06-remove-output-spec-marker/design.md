## Context

The `Output` interface in `output.go` defines the contract for structured output modes (Object, Array, Choice, JSON, Text). It includes an `OutputSpec()` marker method that was intended to seal the interface, following the pattern used by `TextStreamPart`, `ContentPart`, `Message`, and others. However, because the interface is in `aisdk` and all implementations are in the `output/` sub-package, the marker must be exported -- defeating the sealing purpose. This is documented in the code itself.

## Goals / Non-Goals

**Goals:**
- Remove the `OutputSpec()` marker method from the `Output` interface and all implementations
- Update the `structured-output` spec to reflect the new interface contract
- Keep all existing tests passing

**Non-Goals:**
- Restructuring the package layout to enable true sealing (moving `Output` into `output/` or moving implementations into `aisdk`)
- Adding any replacement mechanism for the marker

## Decisions

**Remove the marker entirely rather than making it unexported via package restructuring.**

Moving the interface into `output/` or moving implementations into `aisdk` would enable true sealing with an unexported marker, but both options introduce import cycle risks or break the current clean separation between orchestration (`aisdk`) and output modes (`output/`). The three remaining methods have sufficiently specific signatures that no sealing mechanism is needed.

## Risks / Trade-offs

**[Risk] External code calling `OutputSpec()` directly** -- The method is documented as "not intended to be called directly" and grep confirms zero call sites. Any external caller would be relying on undocumented behavior. Risk is negligible.

**[Trade-off] No sealing mechanism at all** -- Without the marker, the `Output` interface is fully open. This is acceptable because the method signature specificity (particularly `ResponseFormat() *provider.ResponseFormat`) provides equivalent protection against accidental satisfaction.
