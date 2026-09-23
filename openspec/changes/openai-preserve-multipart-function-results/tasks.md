## 1. Baseline and regression evidence

- [x] 1.1 Check registered `@ai-sdk/openai` 4.0.71 source and tests against generic/custom/parallel output and breakpoint scenarios in this spec; record that custom uploaded references warn/drop, not convert.
- [x] 1.2 Inventory existing `test/conformance/openai/recorded/` and pinned `upstream/` plus `INDEX.yaml` for authentic tool-result multipart/cache/reference continuation inputs. If a suitable input exists, update/regenerate its `expected-requests.jsonl` first and confirm request replay fails before fixing Go; do not alter `input*.chunks.txt`. If none exists, record the request-snapshot coverage gap and use focused synthetic provider tests only.
- [x] 1.3 Add failing `providers/openai` request-body tests for generic scalar text/JSON/error/denied with schema quoting, output-vs-result breakpoint precedence and OpenAI/Azure namespaces, plus generic ordered multipart text/image/file URL/bytes/reference (including a non-PDF inline file request), missing namespace and unsupported warnings.
- [x] 1.4 Add failing request-body tests for custom scalar cache and multipart text/data/URL plus exact reference warning/drop, preserving the native output item/call identity; add ordered parallel mixed scalar/multipart child, per-content cache, and scalar breakpoint tests.

## 2. Request conversion

- [x] 2.1 Implement generic typed function output conversion for multipart text, image/file inline bytes/base64, URLs, and active-namespace uploaded references, including full inline media-type resolution without the user-file converter's PDF-only restriction, image detail, per-element breakpoint, ordered omission warnings, and missing-reference error behavior.
- [x] 2.2 Select scalar output-level then tool-result part breakpoints for generic and custom result items while preserving scalar/schema encoding and existing hosted/client tool dispatch, `caller`, and `call_id`.
- [x] 2.3 Switch parallel child serialization to generic output conversion (typed multipart JSON-serialized per child), retain index order and newline contract, and apply only scalar child breakpoints at wrapper level; preserve existing continuation and invalid-group fallback behavior.
- [x] 2.4 Match pinned custom unsupported-reference warning text and drop the reference part; keep supported custom content conversion and content-level cache behavior unchanged.

## 3. Verification

- [x] 3.1 Run focused `providers/openai` request and parallel tests and full OpenAI module tests (`cd providers/openai && go test ./...`); confirm existing hosted-tool/schema regressions still pass.
- [x] 3.2 Run `mise run parity-check`, inspect any changed authentic request snapshots, and report separately whether provider-recorded multipart/reference request coverage was available (synthetic tests do not prove live provider acceptance).
- [x] 3.3 Compare final request shapes and warning/drop behavior to the pinned implementation/tests; confirm no code or docs claim custom uploaded-reference support and no fixture input provenance or public API changed.
