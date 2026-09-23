import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("multi-step part IDs", () => {
  it("remaps reused provider IDs and preserves separate assembled text and reasoning", async () => {
    const response = await fetchScenario("reused-part-ids");
    expect(response.status).toBe(200);
    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }
    for (const type of ["text-start", "text-delta", "text-end", "reasoning-start", "reasoning-delta", "reasoning-end"]) {
      expect(chunks.filter(chunk => chunk.type === type).map(chunk => "id" in chunk ? chunk.id : undefined)).toEqual(["0", "generated"]);
    }
    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({
      stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(chunk => controller.enqueue(chunk)); controller.close(); } }),
      terminateOnError: true,
    })) message = next;
    expect(message?.parts.filter(part => part.type === "text").map(part => part.text)).toEqual(["first answer", "second answer"]);
    expect(message?.parts.filter(part => part.type === "reasoning").map(part => part.text)).toEqual(["first thought", "second thought"]);
  });
});
