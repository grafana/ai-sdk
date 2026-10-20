import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema } from "ai";

const wire = readFileSync(0);
const chunks = [];
for await (const result of parseJsonEventStream({
  stream: new Response(wire).body,
  schema: uiMessageChunkSchema,
})) {
  assert.equal(result.success, true);
  chunks.push(result.value);
}
assert.deepEqual(chunks.map((chunk) => chunk.type), [
  "start", "start-step", "text-start", "text-delta", "text-delta",
  "text-end", "finish-step", "finish",
]);
let message;
for await (const current of readUIMessageStream({
  stream: new ReadableStream({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(chunk);
      controller.close();
    },
  }),
  terminateOnError: true,
})) message = current;
assert.equal(message.role, "assistant");
assert.equal(message.parts.find((part) => part.type === "text").text, "Hello, world!");
console.log("Root consumer SSE parsed and assembled by pinned ai@7.0.127");
