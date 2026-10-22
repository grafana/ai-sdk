# Tasks

- [x] Add request mapping and focused unit tests for system/text history, function tools/results, options, stop reasons, usage, and unsupported blocks.
- [x] Add unary and incremental SSE response mapping with fixed errors and cancellation behavior.
- [x] Wire `/v1/messages` into shared Gateway routing, authentication, catalog execution, and telemetry.
- [x] Add actual-command integration tests using pinned official Anthropic Go and TypeScript SDKs and synthetic providers.
- [x] Document supported subset, auth setup, limits ownership, and compatibility gaps.
- [x] Run focused Go/SDK command tests, module/race/format/check/parity requirements, OpenSpec verification, and independent review; sync and archive OpenSpec artifacts.
- [x] Add inbound protocol conformance replay against unchanged recorded/imported Anthropic fixtures, comparing direct official SDK consumption with Gateway output; retain separately labeled synthetic boundary cases.
