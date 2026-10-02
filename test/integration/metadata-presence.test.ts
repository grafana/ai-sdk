import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { convertToModelMessages, readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("opaque metadata presence", () => {
  it("preserves empty replacements and separate tool scopes through Go SSE and frontend assembly", async () => {
    const response = await fetchScenario("metadata-presence");
    expect(response.status).toBe(200);
    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }
    expect(chunks.find(chunk => chunk.type === "text-delta" && chunk.delta === "")).toMatchObject({ providerMetadata: {} });
    expect(chunks.find(chunk => chunk.type === "tool-input-available")).toMatchObject({ providerMetadata: {} });
    const outputs = chunks.filter(chunk => chunk.type === "tool-output-available");
    expect(outputs.at(-1)).toMatchObject({ providerMetadata: {} });
    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({ stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(chunk => controller.enqueue(chunk)); controller.close(); } }), terminateOnError: true })) message = next;
    expect(message!.parts.find(part => part.type === "text")).toMatchObject({ text: "answer", providerMetadata: {} });
    expect(message!.parts.find(part => part.type === "reasoning")).toMatchObject({ text: "thought", providerMetadata: { future: { new: [null, false, 0, "", [], {}] } } });
    expect(message!.parts.find(part => part.type === "tool-weather")).toMatchObject({ callProviderMetadata: {}, resultProviderMetadata: {} });
    expect(message!.parts.find(part => part.type === "reasoning-file")).toMatchObject({ providerMetadata: {} });
    expect(message!.parts.find(part => part.type === "source-url")).toMatchObject({ providerMetadata: {} });
    const history = await convertToModelMessages([JSON.parse(JSON.stringify(message!))]);
    expect(history[0].content).toEqual(expect.arrayContaining([
      expect.objectContaining({ type: "text", providerOptions: {} }),
      expect.objectContaining({ type: "tool-call", providerOptions: {} }),
    ]));
    expect(history[1].content).toEqual(expect.arrayContaining([expect.objectContaining({ type: "tool-result", providerOptions: {} })]));
  });
});
