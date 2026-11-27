## Design

Reuse the unary JSON replay server and golden projection. The registered provider's `doGenerate` produces expectations against a loopback server; the Go runner calls the adapter's `DoGenerate` with the same call options. A successful unary replay requires exactly one captured request. Do not allow a missing unary input or expectation to fall back to a stream fixture or skip a test.

Import byte-identical inputs from each provider's exact registered Git release tag. Pair each imported response with call options from its upstream test, and use the existing request comparator. Keep all result content, usage, finish reason, metadata and warnings in the existing projection. Do not drop fields or rewrite values to make a fixture pass. The projection excludes debug request/response envelopes; if the reference emits a behavior field it cannot represent, generation must reject it.

The live recorder remains streaming-only. Imported fixtures establish behavior for their inputs, not current API acceptance or exhaustive unary coverage.
