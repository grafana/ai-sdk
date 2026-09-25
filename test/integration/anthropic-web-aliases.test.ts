import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("Anthropic web tool aliases", () => {
  it("preserves custom search and fetch names in SSE and assembled messages", async () => {
    const response = await fetchScenario("anthropic-web-aliases");
    expect(response.status).toBe(200);
    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }
    for (const [id, name] of [["search-1", "search_latest"], ["fetch-1", "fetch_latest"]]) {
      expect(chunks.find(chunk => chunk.type === "tool-input-available" && chunk.toolCallId === id)).toMatchObject({ toolName: name, providerExecuted: true });
      expect(chunks.find(chunk => chunk.type === "tool-output-available" && chunk.toolCallId === id)).toMatchObject({ providerExecuted: true });
    }
    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({
      stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(chunk => controller.enqueue(chunk)); controller.close(); } }),
      terminateOnError: true,
    })) message = next;
    expect(message?.parts.find(part => part.type === "tool-search_latest")).toMatchObject({ toolCallId: "search-1", state: "output-available", input: { query: "Go" } });
    expect(message?.parts.find(part => part.type === "tool-fetch_latest")).toMatchObject({ toolCallId: "fetch-1", state: "output-available", input: { url: "https://example.com" } });
  });
});
