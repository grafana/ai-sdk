import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { streamText } from "../../../tools/node_modules/ai/src/index.ts";

const fixtureDir = dirname(fileURLToPath(import.meta.url));
const chunks = readFileSync(join(fixtureDir, "input.jsonl"), "utf8")
  .trim()
  .split("\n")
  .map((line) => {
    const chunk = JSON.parse(line);
    if (chunk.type === "response-metadata") {
      chunk.timestamp = new Date(chunk.timestamp);
    }
    if (chunk.data?.type === "url") {
      chunk.data.url = new URL(chunk.data.url);
    }
    return chunk;
  });

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
});
const output: string[] = [];
for await (const chunk of result.toUIMessageStream({
  generateMessageId: () => "message-1",
})) {
  output.push(JSON.stringify(chunk));
}
writeFileSync(join(fixtureDir, "expected.jsonl"), `${output.join("\n")}\n`);
