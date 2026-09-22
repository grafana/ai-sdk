import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("upgraded provider tool metadata", () => {
  it("retains wrapper identity and Anthropic caller metadata in chunks and messages", async () => {
    const response = await fetchScenario("upgrade-tool-metadata");
    expect(response.status).toBe(200);
    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }
    const caller = { anthropic: { caller: { type: "code_execution_20260120", toolId: "program_1" } } };
    const wrapper = { openai: { parallelToolCall: { itemId: "fc_parallel", toolCallId: "call_parallel", toolName: "parallel", input: "wrapper-input", index: 0, count: 1 } } };
    expect(chunks.find(chunk => chunk.type === "tool-input-available" && chunk.toolCallId === "call_parallel_0")).toMatchObject({ providerMetadata: wrapper });
    expect(chunks.find(chunk => chunk.type === "tool-output-available" && chunk.toolCallId === "search_1")).toMatchObject({ providerMetadata: caller, providerExecuted: true });
    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({
      stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(chunk => controller.enqueue(chunk)); controller.close(); } }),
      terminateOnError: true,
    })) message = next;
    expect(message?.parts.find(part => part.type === "tool-weather")).toMatchObject({ toolCallId: "call_parallel_0", callProviderMetadata: wrapper, input: { location: "SF" } });
    expect(message?.parts.find(part => part.type === "tool-web_search")).toMatchObject({ toolCallId: "search_1", callProviderMetadata: caller, resultProviderMetadata: caller, output: [] });
  });
});
