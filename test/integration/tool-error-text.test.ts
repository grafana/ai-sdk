import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { convertToModelMessages, readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

const failure = "weather backend timeout";
const unavailable = "AI_NoSuchToolError: Model tried to call unavailable tool 'missing'. Available tools: weather.";

async function scenarioChunks(mode: string): Promise<UIMessageChunk[]> {
  const response = await fetchScenario(`tool-error-text?mode=${mode}`);
  expect(response.status).toBe(200);
  const parsed = parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema });
  const chunks: UIMessageChunk[] = [];
  for await (const result of parsed) {
    if (!result.success) throw result.error;
    chunks.push(result.value);
  }
  return chunks;
}

async function assembleMessage(chunks: UIMessageChunk[]): Promise<UIMessage> {
  const stream = new ReadableStream<UIMessageChunk>({ start(controller) {
    for (const chunk of chunks) controller.enqueue(chunk);
    controller.close();
  } });
  let message: UIMessage | undefined;
  for await (const next of readUIMessageStream({ stream, terminateOnError: true })) message = next;
  return message!;
}

function errorChunks(chunks: UIMessageChunk[]) {
  return chunks.filter(chunk => chunk.type === "tool-input-error" || chunk.type === "tool-output-error");
}

describe("tool error text", () => {
  it.each(["default", "inactive", "inactive-blank"])("keeps the model's error text so %s history round-trips", async (mode) => {
    const chunks = await scenarioChunks(mode);
    const unavailableChunk = {
      type: "tool-input-error", toolCallId: "missing-1", toolName: "missing",
      input: mode === "inactive-blank" ? " \t" : {}, errorText: unavailable,
      ...(mode === "default" ? { dynamic: true } : {}),
    };
    expect(chunks.filter(chunk => "toolCallId" in chunk && chunk.toolCallId === "missing-1")).toEqual([unavailableChunk]);
    expect(errorChunks(chunks)).toEqual([
      unavailableChunk,
      { type: "tool-output-error", toolCallId: "weather-1", errorText: failure },
    ]);

    const message = await assembleMessage(chunks);
    if (mode === "inactive-blank") {
      expect(message.parts.find(part => "toolCallId" in part && part.toolCallId === "missing-1")).toMatchObject({
        state: "output-error", input: " \t",
      });
    }
    const modelReceived = JSON.parse(message.parts.filter(part => part.type === "text").map(part => part.text).join(""));
    expect(modelReceived).toEqual({ "weather-1": failure, "missing-1": unavailable });

    const toolMessages = (await convertToModelMessages([message])).filter(m => m.role === "tool");
    const outputs = Object.fromEntries(toolMessages.flatMap(m => m.content).flatMap(part =>
      part.type === "tool-result" && part.output.type === "error-text" ? [[part.toolCallId, part.output.value]] : []));
    expect(outputs).toEqual(modelReceived);
  });

  it("redacts tool errors once when an error callback is set", async () => {
    const chunks = await scenarioChunks("callback");
    expect(errorChunks(chunks)).toEqual([
      { type: "tool-input-error", toolCallId: "missing-1", toolName: "missing", input: {}, dynamic: true, errorText: "redacted" },
      { type: "tool-output-error", toolCallId: "weather-1", errorText: "redacted" },
    ]);
  });

  it("limits the callback to stream errors when redaction is disabled", async () => {
    const chunks = await scenarioChunks("callback-unredacted");
    expect(errorChunks(chunks).map(chunk => "errorText" in chunk && chunk.errorText)).toEqual([unavailable, failure]);
  });
});
