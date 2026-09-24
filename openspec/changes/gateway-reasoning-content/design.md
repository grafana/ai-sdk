# Reasoning transport contract

## Metadata decision

The same projection applies to unary reasoning text/files and reasoning start/delta/end/file events. Fields remain at their original event position; the Gateway never accumulates or merges metadata.

| Namespace | Keys | Values | Replay |
| --- | --- | --- | --- |
| anthropic | signature, redactedData | JSON strings, including empty | Same keys in part providerOptions |
| amazonBedrock / bedrock | signature, redactedData, redactedContent | JSON strings, including empty; namespace preserved | Same keys in part providerOptions |
| openai | itemId | JSON string, including empty | Same key in part providerOptions |
| openai | reasoningEncryptedContent | JSON string or null, including empty | Same key in part providerOptions |

Absent metadata stays absent. A present empty object stays `{}` and replaces prior metadata in assembled history; omitted metadata preserves it. Unknown namespaces/keys are omitted, including backend identity, headers, credentials and operational details. A present object filtered to nothing remains `{}`. Known fields with invalid types fail adaptation. Raw input metadata is bounded before decoding, including namespace cardinality, UTF-8 and field sizes; the complete encoded response/frame is independently bounded. Metadata is continuation payload, never telemetry. File transport supports this same bounded projection; arbitrary file namespaces such as the upstream core's `testProvider` witness are intentionally excluded at the service boundary, while core itself retains opaque metadata.

## Lifecycle

Each reasoning ID has independent active state. Concurrent reasoning blocks are accepted. Text and reasoning may share an ID; tools retain their own state. Atomic reasoning files may appear between any content events. IDs are never rewritten by ProviderWire. Same-family reuse after end remains rejected as an explicit bounded service rule, independently from upstream high-level ID reservation/remapping. Finish requires all reasoning and text/tool blocks closed. Response metadata must precede any content. Nonterminal provider errors do not close active reasoning. Existing cancellation, counters, drain and write-failure rules remain in force.

## Scope and residual retry risk

Reasoning-bearing fallback prompts remain outside the text replay subset; reasoning output commits an attempt. Valid reasoning-only success must produce one physical call under default high-level retries. Malformed/oversized paid output still uses the existing retryable canonical 500; narrow post-provider error reclassification is deferred, not silently changed here. No exactly-once claim is made.

## High-level host composition and frontend presence

Pinned `ai.generateText` inserts an `ai/7.0.107` user-agent into provider-call headers. The strict service still rejects arbitrary nonempty body headers. Acceptance tests deliberately compose a host `wrapLanguageModel` adapter that verifies the sole SDK-owned header and moves it to the outer Gateway HTTP headers, alongside provider-level host authentication. It neither forwards that value to native providers nor enables WP21. Bare unadapted `generateText` remains incompatible with the strict body-header subset; tests do not claim otherwise. Pinned `streamText` and the independent Go client use their supported paths directly.

Reasoning UI chunks preserve non-nil empty metadata maps in JSON. Omitting `{}` would make the pinned frontend retain an old signature instead of clearing it. Frontend tests schema-parse actual Go SSE and assemble concurrent reasoning/text plus an atomic empty-data reasoning file.

Native Anthropic replay distinguishes absent/null from present-empty signature and redactedData. Unary thinking response metadata also retains a present empty signature using the underlying native JSON presence flag. Existing unsigned-reasoning compatibility behavior is not broadened into a claim of complete native parity.

## Integration checkpoints

- #191 at `d50fba49ccfb322c2a8040f6e96909b3a30d0cf1`: reconcile scoped options; do not enable root options/headers.
- #238 at `4e8b990ee7ca99492095b715e8641e2ea42bb3f5` replaces #237 at `9253a2ba52bcfbe2e2bd709c6fac711571afd4c3`: reconcile independent decoding, metadata projection, boolean fields, SSE and published pins; provider tools remain unsupported.
- #164 at `d371a4346df3d57f460a497c2bcb4e9540332f7c`: optional reasoning-summary suppression is distinct from transport and is not a prerequisite.
- #201 at `5fabd08e621e3482cac95438267b36c7755b822e`: rerun reasoning rows if present; unrelated advisory matrix failures are not WP17 scope.

Exact upstream reference: `08ae5ad05bc12496dd1ffcf64e34419e0831300d`, including ai 7.0.107, provider 4.0.17, Gateway 4.0.87. WP15 active artifacts remain untouched.
