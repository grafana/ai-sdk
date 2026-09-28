## 1. Pin expectations and build deterministic fixtures

- [x] 1.1 Confirm registered versions and inspect pinned `callCompletionApi`, `AbstractChat`/transport and hook tests for exact overlap, regenerate, reconnect, abort and callback outcomes before fixing assertions.
- [x] 1.2 Add request-scoped Go UI-stream scenarios for distinguishable regenerate responses, metadata/data updates, and fixed-ID reconnect GET outcomes (stream, 204, HTTP error); ensure the GET handler is test-only, bounded and cancellation-aware, with no durable storage.
- [x] 1.3 Add Go text-stream scenarios to distinguish concurrent completion requests and to provide observable partial JSON followed by a deterministic body-read failure; reuse existing abortable stream and HTTP-error handlers where possible.

## 2. Chat hook integration

- [x] 2.1 Extend the actual `useChat` probe to record immutable messages/IDs/parts/metadata, status, `onData`, `onFinish`, `onError`, and expose regenerate/resume actions without losing existing tests.
- [x] 2.2 Test regeneration retaining user identity and replacing the earlier assistant response; exercise deterministic reconnect stream, no-stream 204, and error paths with exact state/identity/callback assertions.
- [x] 2.3 Test chat abort/error callback flags and counts, message metadata and data updates; assert `onData` sees retained and transient data while transient data does not enter message parts. Parse any new UI SSE with `parseJsonEventStream` + `uiMessageChunkSchema` and assert assembled messages.

## 3. Completion and object hook integration

- [x] 3.1 Extend `useCompletion` probes/tests with distinguishable concurrent prompts and request-local phases; assert earlier `onFinish` receives its prompt/result or distinguishable earlier failure reaches `onError`, without earlier settlement overwriting the newer displayed completion/error or clearing its active loading, including both earlier success and failure while the later request is active.
- [x] 3.2 Extend `useObject` probe with error/loading/stop/onError histories and add HTTP failure, failed body read after partial value, stop with retained partial value, and successful onFinish assertions; keep normal schema-failure case separate from transport failure.

## 4. Validation and coverage boundary

- [x] 4.1 Run `mise run test-integration` (including repeated overlap/cancellation tests where practical), review Go HTTP stream shutdown and frontend assertions for deterministic request-local behavior.
- [x] 4.2 Run `mise run parity-check` if committed parity behavior/fixtures change, verify any new provider inputs have valid provenance (none planned), and keep React hooks/UI coverage `mixed` until breadth is established; document any newly discovered proof gap without silently adding a runtime API.
