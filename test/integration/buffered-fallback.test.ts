import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("buffered cross-model fallback", () => {
  it("assembles only the successful backup response", async () => {
    const response = await fetchScenario("buffered-model-fallback");
    expect(response.status).toBe(200);
    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }
    expect(chunks.filter(chunk => chunk.type === "error")).toEqual([]);
    expect(chunks.filter(chunk => chunk.type === "start-step")).toHaveLength(1);
    expect(chunks.filter(chunk => chunk.type === "text-delta")).toEqual([
      { type: "text-delta", id: "answer", delta: "Haiku decision" },
    ]);
    expect(chunks.at(-1)).toMatchObject({ type: "finish", finishReason: "stop" });
    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({
      stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(chunk => controller.enqueue(chunk)); controller.close(); } }),
    })) message = next;
    expect(message?.parts.filter(part => part.type === "text")).toEqual([
      expect.objectContaining({ text: "Haiku decision", state: "done" }),
    ]);
  });
});
