import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("local preliminary tool outputs", () => {
  it("validates preliminary/final chunks and assembles only the final output", async () => {
    const response = await fetchScenario("local-preliminary");
    expect(response.status).toBe(200);
    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }
    const outputs = chunks.filter(chunk => chunk.type === "tool-output-available");
    expect(outputs).toMatchObject([
      { type: "tool-output-available", toolCallId: "local-1", output: { status: "loading" }, preliminary: true },
      { type: "tool-output-available", toolCallId: "local-1", output: { status: "done" }, preliminary: true },
      { type: "tool-output-available", toolCallId: "local-1", output: { status: "done" } },
    ]);
    expect(outputs[2]).not.toHaveProperty("preliminary");
    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({
      stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(chunk => controller.enqueue(chunk)); controller.close(); } }),
      terminateOnError: true,
    })) message = next;
    expect(message?.parts.find(part => part.type === "tool-lookup")).toMatchObject({
      toolCallId: "local-1", state: "output-available", output: { status: "done" },
    });
  });
});
