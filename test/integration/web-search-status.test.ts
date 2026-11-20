import { it, expect } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

it("keeps a non-completed web search as a provider tool error in the UI", async () => {
  const response = await fetchScenario("web-search-status");
  expect(response.status).toBe(200);

  const chunks: UIMessageChunk[] = [];
  const parsed = parseJsonEventStream({
    stream: response.body!,
    schema: uiMessageChunkSchema,
  });
  for await (const part of parsed) {
    if (!part.success) throw part.error;
    chunks.push(part.value);
  }

  const error = {
    toolCallId: "ws_1",
    providerExecuted: true,
    errorText: '{"status":"searching"}',
  };
  expect(chunks.filter(chunk => chunk.type === "tool-output-error"))
    .toEqual([{ type: "tool-output-error", ...error }]);
  expect(chunks.filter(chunk => chunk.type === "tool-output-available" || chunk.type === "error"))
    .toEqual([]);

  const stream = new ReadableStream<UIMessageChunk>({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(chunk);
      controller.close();
    },
  });

  let parts;
  for await (const message of readUIMessageStream({ stream, terminateOnError: true })) {
    parts = message.parts;
  }
  expect(parts).toContainEqual(expect.objectContaining({
    type: "tool-lookup",
    state: "output-error",
    input: {},
    ...error,
  }));
});
