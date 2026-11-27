## Context

The provider implementation coverage is mixed. The reference for this change is the package tag `@ai-sdk/amazon-bedrock@5.0.146` at `3dad57f204c7332178d28c3467c7311875b66a3e`, not the later umbrella `ai@7.0.179` commit. In [`doGenerate`](https://github.com/vercel/ai/blob/3dad57f204c7332178d28c3467c7311875b66a3e/packages/amazon-bedrock/src/amazon-bedrock-chat-language-model.ts#L827-L851), direct text takes precedence whenever it is not nullish; otherwise each non-nullish citation text fragment is emitted separately. The same stateful extractor handles all text when the JSON instruction route is active.

## Goals / Non-Goals

Restore the unary text parts with the same ordering, without changing the interpretation of reasoning, tools, or finish reasons. Do not change prompt history, streaming, Mantle, or surface citation annotations as sources or metadata.

## Decisions

Use optional string pointers in the private response types so decoding can distinguish `""` from missing or null. Emit direct text or the ordered citation fragments before converting any other fields in the response block. Both forms use the same extraction step, retaining one content part per input string even when the extracted text is empty.

The pin's test file has inline mocked responses for citation text (lines 4321–4444) and empty text between reasoning blocks (lines 7568–7643). These cases have no corresponding upstream fixture or local recording. Test them with a synthetic transport through public `DoGenerate`; this proves adapter behavior, not live AWS response shapes. Existing recordings and imports remain unchanged.
