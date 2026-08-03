import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { streamText, tool } from "../../tools/node_modules/ai/src/index.ts";
import { z } from "../../tools/node_modules/zod/index.js";

const fixtureDir = dirname(fileURLToPath(import.meta.url));
const chunks = readFileSync(join(fixtureDir, "input.jsonl"), "utf8")
  .trim()
  .split("\n")
  .map((line) => JSON.parse(line));

const model = {
  specificationVersion: "v4",
  provider: "test",
  modelId: "test-model",
  supportedUrls: {},
  doStream: async () => ({
    stream: new ReadableStream({
      start(controller) {
        for (const chunk of chunks) controller.enqueue(chunk);
        controller.close();
      },
    }),
  }),
};

const result = streamText({
  model: model as never,
  prompt: "test",
  maxRetries: 0,
  tools: {
    web_search: tool({
      inputSchema: z.object({ query: z.string() }),
    }),
  },
});
const output: string[] = [];
for await (const chunk of result.toUIMessageStream({
  generateMessageId: () => "message-1",
})) {
  output.push(JSON.stringify(chunk));
}
writeFileSync(join(fixtureDir, "expected.jsonl"), `${output.join("\n")}\n`);
