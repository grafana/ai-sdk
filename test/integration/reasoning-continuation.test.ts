import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("WP17 reasoning continuation", () => {
  it("schema-parses concurrent Go blocks, replaces metadata and assembles an atomic empty file", async () => {
    const response = await fetchScenario("reasoning-continuation");
    expect(response.status).toBe(200);
    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }
    expect(chunks.filter(part => part.type === "reasoning-start").map(part => part.id)).toEqual(["same", "other"]);
    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({ stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(part => controller.enqueue(part)); controller.close(); } }), terminateOnError: true })) message = next;
    const reasoning = message!.parts.filter(part => part.type === "reasoning");
    expect(reasoning.map(part => part.text)).toEqual(["one", "two"]);
    expect(reasoning.map(part => part.providerMetadata)).toEqual([{ anthropic: { signature: "final" } }, {}]);
    const file = message!.parts.find(part => part.type === "reasoning-file");
    expect(file).toMatchObject({ type: "reasoning-file", mediaType: "image/png", url: "data:image/png;base64,", providerMetadata: { anthropic: { signature: "final" } } });
    expect(message!.parts.filter(part => part.type === "text").map(part => part.text)).toEqual(["answer"]);
  });
});
