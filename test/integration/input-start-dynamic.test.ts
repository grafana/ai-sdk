import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { jsonSchema, readUIMessageStream, streamText, uiMessageChunkSchema, type UIMessageChunk } from "ai";
import { getServerUrl } from "./helpers.js";

const tools = {
  dynamic: { type: "dynamic" as const, inputSchema: jsonSchema({ type: "object" as const }) },
  ordinary: { inputSchema: jsonSchema({ type: "object" as const }) },
};

function pinnedModel(name: string, dynamic: boolean | undefined) {
  const start = { type: "tool-input-start", id: "call-1", toolName: name, ...(dynamic === undefined ? {} : { dynamic }) };
  return {
    specificationVersion: "v4",
    provider: "test",
    modelId: "input-start-dynamic",
    supportedUrls: {},
    doGenerate: async () => { throw new Error("unary not used"); },
    doStream: async () => ({ stream: new ReadableStream({
      start(controller) {
        controller.enqueue({ type: "stream-start", warnings: [] });
        controller.enqueue(start);
        controller.enqueue({ type: "tool-input-end", id: "call-1" });
        controller.enqueue({ type: "finish", finishReason: { unified: "stop" }, usage: { inputTokens: {}, outputTokens: {} } });
        controller.close();
      },
    }) }),
  } as unknown as Parameters<typeof streamText>[0]["model"];
}

async function pinnedStart(name: string, dynamic: boolean | undefined, ui: boolean) {
  const result = streamText({ model: pinnedModel(name, dynamic), prompt: "test", tools });
  const parts: unknown[] = [];
  for await (const part of ui ? result.toUIMessageStream() : result.fullStream) {
    if (part.type === "tool-input-start") parts.push(part);
  }
  expect(parts).toHaveLength(1);
  return parts[0] as { dynamic?: boolean };
}

async function goChunks(name: string, dynamic: boolean | undefined) {
  const query = new URLSearchParams({ tool: name, ...(dynamic === undefined ? {} : { dynamic: String(dynamic) }) });
  const response = await fetch(`${getServerUrl()}/scenario/input-start-dynamic?${query}`, { method: "POST" });
  expect(response.status).toBe(200);
  const parsed = parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema });
  const chunks: UIMessageChunk[] = [];
  for await (const part of parsed) {
    expect(part.success).toBe(true);
    if (part.success) chunks.push(part.value);
  }
  const stream = new ReadableStream<UIMessageChunk>({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(chunk);
      controller.close();
    },
  });
  const messages = [];
  for await (const message of readUIMessageStream({ stream, terminateOnError: true })) messages.push(message);
  expect(messages.at(-1)?.role).toBe("assistant");
  return chunks;
}

describe("input-start dynamic across text and UI streams", () => {
  for (const tc of [
    { name: "dynamic", marker: undefined, text: true, ui: true },
    { name: "dynamic", marker: false, text: false, ui: true },
    { name: "dynamic", marker: true, text: true, ui: true },
    { name: "ordinary", marker: undefined, text: false, ui: undefined },
    { name: "unknown", marker: false, text: false, ui: false },
    { name: "unknown", marker: true, text: true, ui: true },
  ]) {
    it(`${tc.name} with ${String(tc.marker)} marker matches pinned orchestration and parsed Go SSE`, async () => {
      const text = await pinnedStart(tc.name, tc.marker, false);
      expect(text.dynamic).toBe(tc.text);
      const pinnedUI = await pinnedStart(tc.name, tc.marker, true);
      const chunks = await goChunks(tc.name, tc.marker);
      const goUI = chunks.find((chunk) => chunk.type === "tool-input-start") as { dynamic?: boolean } | undefined;
      expect(goUI).toBeDefined();
      expect(pinnedUI.dynamic).toBe(tc.ui);
      expect(goUI?.dynamic).toBe(pinnedUI.dynamic);
      expect(goUI && "dynamic" in goUI).toBe("dynamic" in pinnedUI);
    });
  }
});
