import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

async function scenarioChunks(name: string): Promise<UIMessageChunk[]> {
  const response = await fetchScenario(name);
  expect(response.status).toBe(200);
  const parsed = parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema });
  const chunks: UIMessageChunk[] = [];
  for await (const result of parsed) {
    if (!result.success) throw result.error;
    chunks.push(result.value);
  }
  return chunks;
}

async function assembleMessage(chunks: UIMessageChunk[]): Promise<UIMessage | undefined> {
  const stream = new ReadableStream<UIMessageChunk>({ start(controller) {
    for (const chunk of chunks) controller.enqueue(chunk);
    controller.close();
  } });
  let message: UIMessage | undefined;
  for await (const next of readUIMessageStream({ stream, terminateOnError: true })) message = next;
  return message;
}

describe("deferred tool discovery", () => {
  it.each(["static", "provider", "unknown"])("assembles a single %s provider-driven part before discovery", async kind => {
    const chunks = await scenarioChunks(`deferred-provider-ui?kind=${kind}`);
    for (const chunk of chunks) {
      if (chunk.type === "tool-input-start" || chunk.type === "tool-input-available" || chunk.type === "tool-output-available") {
        expect(chunk.dynamic).toBe(kind === "unknown" ? true : undefined);
        expect(chunk.providerExecuted).toBe(true);
      }
    }
    const message = await assembleMessage(chunks);
    const tools = message?.parts.filter(part => "toolCallId" in part);
    expect(tools).toHaveLength(1);
    expect(tools?.[0]).toMatchObject({
      type: kind === "unknown" ? "dynamic-tool" : "tool-web",
      toolCallId: "provider", state: "output-available", input: {}, output: "sunny", providerExecuted: true,
    });
  });
  it("preserves static errors and next-step tool outcomes in assembled messages", async () => {
    const chunks = await scenarioChunks("deferred-tool-discovery");
    expect(chunks.filter(chunk => chunk.type === "tool-input-error")).toEqual([
      { type: "tool-input-error", toolCallId: "too-early", toolName: "getWeather", input: {}, errorText: "An error occurred." },
    ]);
    expect(chunks.filter(chunk => chunk.type === "tool-output-error")).toEqual([
      { type: "tool-output-error", toolCallId: "too-early", errorText: "An error occurred." },
    ]);
    expect(chunks.filter(chunk => chunk.type === "tool-output-available")).toEqual([
      { type: "tool-output-available", toolCallId: "search", output: { tools: [{ name: "getWeather", description: "Weather forecast" }] } },
      { type: "tool-output-available", toolCallId: "weather", output: "sunny" },
    ]);
    expect(chunks.filter(chunk => chunk.type === "start-step")).toHaveLength(3);
    expect(chunks.filter(chunk => chunk.type === "finish-step")).toHaveLength(3);
    const searchIndex = chunks.findIndex(chunk => chunk.type === "tool-output-available" && chunk.toolCallId === "search");
    const weatherIndex = chunks.findIndex(chunk => chunk.type === "tool-input-available" && chunk.toolCallId === "weather");
    expect(chunks.slice(searchIndex + 1, weatherIndex).map(chunk => chunk.type)).toEqual(["finish-step", "start-step"]);
    expect(chunks.at(-1)).toEqual({ type: "finish", finishReason: "stop" });

    const message = await assembleMessage(chunks);
    expect(message?.role).toBe("assistant");
    expect(message?.parts.find(part => part.type === "tool-search")).toMatchObject({
      type: "tool-search", toolCallId: "search", state: "output-available", input: { query: "weather" }, output: { tools: [{ name: "getWeather", description: "Weather forecast" }] },
    });
    expect(message?.parts.find(part => part.type === "tool-getWeather" && part.toolCallId === "too-early")).toMatchObject({
      type: "tool-getWeather", toolCallId: "too-early", state: "output-error", errorText: "An error occurred.",
    });
    expect(message?.parts.find(part => part.type === "tool-getWeather" && part.toolCallId === "weather")).toMatchObject({
      type: "tool-getWeather", toolCallId: "weather", state: "output-available", output: "sunny",
    });
    expect(message?.parts.filter(part => part.type === "text")).toMatchObject([{ type: "text", text: "sunny", state: "done" }]);
  });
});
