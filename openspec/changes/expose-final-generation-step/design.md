## Context

The registered upstream `ai@7.0.173` (`0e8e0543`) selects `steps.at(-1)` in both result types. Its stream promise can reject when no output was generated or a continuation fails. Go already retains recorded history on streaming errors and cancellation, and its two collected entry points return no result on generation errors.

## Goals / Non-Goals

- Expose per-step data without repeating aggregate collection or changing step recording, callbacks, or error handling.
- Keep result ownership changes local to the new accessor.

## Decisions

- Return `(StepResult, bool)` instead of a possibly nil pointer or a new error. The bool indicates a recorded step, not generation success. Stream callers use existing completion and drain semantics; the accessor never takes over the single stream reader.
- Return the selected step by value instead of exposing a pointer to a stored step. Match the existing result views by sharing nested data as read-only; callers can copy nested data before modifying it. An independent deep copy would require a new ownership model for `Steps` data and a separate rule for opaque Go values.

## Risks / Trade-offs

- The accessor cannot enforce read-only access to shared nested Go slices, maps, and pointers. Its documentation makes the copy boundary explicit.
- Stream failure and backpressure follow Go's existing result model, rather than mirroring promise rejection and automatic consumption. The existing stream consumer remains responsible for draining events.
