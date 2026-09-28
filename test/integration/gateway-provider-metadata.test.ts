import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("Gateway metadata across frontend wire", () => {
  it("keeps approved text and tool metadata in chunks and assembled messages", async () => {
    const response = await fetchScenario("gateway-provider-metadata");
    expect(response.status).toBe(200);
    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }
    expect(chunks.map(chunk => chunk.type)).toEqual(["start", "start-step", "text-start", "text-delta", "text-end", "tool-input-available", "tool-output-available", "finish-step", "finish"]);
    const itemId = { openai: { itemId: "msg_1" } };
    const caller = { anthropic: { caller: { type: "direct" } } };
    expect(chunks.find(chunk => chunk.type === "text-start")).toMatchObject({ id: "msg_1", providerMetadata: itemId });
    expect(chunks.find(chunk => chunk.type === "text-end")).toMatchObject({ id: "msg_1", providerMetadata: itemId });
    expect(chunks.find(chunk => chunk.type === "tool-input-available")).toMatchObject({ toolCallId: "call", providerMetadata: caller });
    expect(chunks.find(chunk => chunk.type === "tool-output-available")).toMatchObject({ toolCallId: "call", providerMetadata: caller });
    expect(JSON.stringify(chunks)).not.toContain("private-key");
    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({
      stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(chunk => controller.enqueue(chunk)); controller.close(); } }),
      terminateOnError: true,
    })) message = next;
    expect(message?.parts.find(part => part.type === "text")).toMatchObject({ text: "hello", providerMetadata: itemId });
    expect(message?.parts.find(part => part.type === "tool-weather")).toMatchObject({
      toolCallId: "call", state: "output-available", callProviderMetadata: caller, resultProviderMetadata: caller,
    });
  });
});
