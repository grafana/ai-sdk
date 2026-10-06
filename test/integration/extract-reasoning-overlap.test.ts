import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("extract reasoning from overlapping text blocks", () => {
  it("preserves text and reasoning IDs through SSE parsing and message assembly", async () => {
    const response = await fetchScenario("extract-reasoning-overlap");
    expect(response.status).toBe(200);

    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }

    const content = chunks.filter(chunk =>
      ["text-start", "text-delta", "text-end", "reasoning-start", "reasoning-delta", "reasoning-end"].includes(chunk.type),
    );
    expect(content.map(chunk => [chunk.type, "id" in chunk ? chunk.id : undefined])).toEqual([
      ["reasoning-start", "reasoning-0"], ["reasoning-delta", "reasoning-0"],
      ["reasoning-start", "reasoning-1"], ["reasoning-delta", "reasoning-1"],
      ["reasoning-delta", "reasoning-0"], ["reasoning-end", "reasoning-0"],
      ["text-start", "a"], ["text-delta", "a"],
      ["reasoning-delta", "reasoning-1"], ["reasoning-end", "reasoning-1"],
      ["text-start", "b"], ["text-delta", "b"],
      ["text-end", "a"], ["text-end", "b"],
    ]);
    expect(content.filter(chunk => chunk.type === "reasoning-delta").map(chunk => chunk.delta)).toEqual(["A", "B", "1", "2"]);
    expect(content.filter(chunk => chunk.type === "text-delta").map(chunk => chunk.delta)).toEqual(["Alpha.", "Beta."]);

    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({
      stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(chunk => controller.enqueue(chunk)); controller.close(); } }),
      terminateOnError: true,
    })) message = next;
    expect(message?.parts.filter(part => part.type === "text").map(part => part.text)).toEqual(["Alpha.", "Beta."]);
    expect(message?.parts.filter(part => part.type === "reasoning").map(part => part.text)).toEqual(["A1", "B2"]);
  });
});
