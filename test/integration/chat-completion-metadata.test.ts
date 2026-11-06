import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("Chat completion metadata", () => {
  it("keeps accumulated metadata out of UI content and history", async () => {
    const response = await fetchScenario("chat-completion-metadata");
    expect(response.status).toBe(200);
    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }
    expect(chunks.some(chunk => chunk.type === "custom")).toBe(false);
    expect(chunks.some(chunk => chunk.type === "error")).toBe(false);
    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({
      stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(chunk => controller.enqueue(chunk)); controller.close(); } }),
      terminateOnError: true,
    })) message = next;
    expect(message?.parts.filter(part => part.type !== "step-start")).toEqual([
      expect.objectContaining({ type: "text", text: "hello world" }),
    ]);
  });
});
