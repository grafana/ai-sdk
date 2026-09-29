import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("OpenAI async tool metadata", () => {
  it("parses true, explicit false, and absent metadata through continuation", async () => {
    const response = await fetchScenario("openai-async-metadata");
    expect(response.status).toBe(200);
    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }
    const expected = [
      { id: "call_true", name: "lookup_true", metadata: { itemId: "fc_true", async: true, caller: { type: "program", callerId: "prog_1" } } },
      { id: "call_false", name: "lookup_false", metadata: { itemId: "fc_false", async: false } },
      { id: "call_absent", name: "lookup_absent", metadata: { itemId: "fc_absent" } },
    ];
    for (const { id, name, metadata } of expected) {
      const chunk = chunks.find(chunk => chunk.type === "tool-input-available" && chunk.toolCallId === id);
      expect(chunk).toMatchObject({ toolName: name });
      expect(chunk && "providerMetadata" in chunk ? chunk.providerMetadata?.openai : undefined).toEqual(metadata);
    }
    expect(chunks.filter(chunk => chunk.type === "start-step")).toHaveLength(2);
    expect(chunks.some(chunk => chunk.type === "text-delta" && chunk.delta === "continued")).toBe(true);
    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({
      stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(chunk => controller.enqueue(chunk)); controller.close(); } }),
      terminateOnError: true,
    })) message = next;
    for (const { id, name, metadata } of expected) {
      const part = message?.parts.find(part => part.type === `tool-${name}`);
      expect(part).toMatchObject({ toolCallId: id, state: "output-available", output: { ok: true } });
      expect(part && "callProviderMetadata" in part ? part.callProviderMetadata?.openai : undefined).toEqual(metadata);
    }
    expect(message?.parts.find(part => part.type === "text")).toMatchObject({ text: "continued" });
  });
});
