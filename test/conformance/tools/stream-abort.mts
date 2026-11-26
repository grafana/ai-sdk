import { writeFileSync } from "node:fs";
import { streamText } from "ai";
import { MockLanguageModelV4 } from "ai/test";
import type { LanguageModelV4StreamPart } from "@ai-sdk/provider";

for (const name of ["no-output", "partial-output", "pending-output"] as const) {
  const abortController = new AbortController();
  const abort = () => abortController.abort("manual abort");
  const model = new MockLanguageModelV4({
    doStream: async () => {
      const stream = new ReadableStream<LanguageModelV4StreamPart>({
        start(controller) {
          controller.enqueue({ type: "stream-start", warnings: [] });
          if (name !== "no-output") {
            controller.enqueue({ type: "text-start", id: "t1" });
            controller.enqueue({
              type: "text-delta",
              id: "t1",
              delta: name === "partial-output" ? "partial" : "discarded",
            });
          }
        },
        pull() {
          if (name === "no-output") abort();
        },
      }, { highWaterMark: 0 });
      if (name === "pending-output") abort();
      return { stream };
    },
  });

  const result = streamText({
    model,
    prompt: "hi",
    abortSignal: abortController.signal,
    maxRetries: 0,
  });
  const lines = [];
  for await (const chunk of result.toUIMessageStream()) {
    lines.push(JSON.stringify(chunk));
    if (name === "partial-output" && chunk.type === "text-delta") abort();
  }
  writeFileSync(new URL(`../ui/stream-abort/${name}/expected.jsonl`, import.meta.url),
    lines.join("\n") + "\n");
}
