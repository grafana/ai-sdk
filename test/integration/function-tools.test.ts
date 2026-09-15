import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

async function readFunctionToolScenario(name: string) {
  const response = await fetchScenario(name);
  expect(response.status).toBe(200);
  const chunks: UIMessageChunk[] = [];
  for await (const parsed of parseJsonEventStream({
    stream: response.body!,
    schema: uiMessageChunkSchema,
  })) {
    expect(parsed.success).toBe(true);
    if (parsed.success) chunks.push(parsed.value);
  }

  const messages: UIMessage[] = [];
  for await (const message of readUIMessageStream({
    stream: new ReadableStream<UIMessageChunk>({
      start(controller) {
        for (const chunk of chunks) controller.enqueue(chunk);
        controller.close();
      },
    }),
    terminateOnError: true,
  })) {
    messages.push(message);
  }
  return { chunks, message: messages.at(-1)! };
}

describe("function tool input and continuation", () => {
  it("assembles streamed arguments, executed results, and the follow-up answer", async () => {
    const { chunks, message } = await readFunctionToolScenario("function-tool-input");
    expect(chunks).toContainEqual(expect.objectContaining({
      type: "tool-input-start", toolCallId: "call-1", toolName: "read_evidence", title: "Read evidence",
    }));
    expect(chunks.filter(chunk => chunk.type === "tool-input-delta")).toEqual([
      { type: "tool-input-delta", toolCallId: "call-1", inputTextDelta: "" },
      { type: "tool-input-delta", toolCallId: "call-1", inputTextDelta: '{"service":"checkout"}' },
    ]);
    expect(chunks).toContainEqual(expect.objectContaining({
      type: "tool-input-available", toolCallId: "call-1", toolName: "read_evidence", input: { service: "checkout" },
    }));
    expect(chunks).toContainEqual(expect.objectContaining({
      type: "tool-output-available", toolCallId: "call-1", output: { errorRate: 4.2 },
    }));
    expect(chunks.filter(chunk => chunk.type === "start-step")).toHaveLength(2);
    expect(chunks).not.toContainEqual(expect.objectContaining({ type: "error" }));
    expect(message.parts).toContainEqual(expect.objectContaining({
      type: "tool-read_evidence", toolCallId: "call-1", state: "output-available",
      input: { service: "checkout" }, output: { errorRate: 4.2 },
    }));
    expect(message.parts).toContainEqual(expect.objectContaining({
      type: "text", text: "Checkout error rate is 4.2%.", state: "done",
    }));
  });

  it("reports malformed argument strings as tool input errors and resumes with their results", async () => {
    const { chunks, message } = await readFunctionToolScenario("function-tool-invalid-input");
    expect(chunks).toContainEqual(expect.objectContaining({
      type: "tool-input-error", toolCallId: "call-1", toolName: "read_evidence", errorText: expect.any(String),
    }));
    expect(chunks).not.toContainEqual(expect.objectContaining({ type: "tool-output-available" }));
    expect(chunks).not.toContainEqual(expect.objectContaining({ type: "error" }));
    expect(message.parts).toContainEqual(expect.objectContaining({
      type: "tool-read_evidence", toolCallId: "call-1", state: "output-error", errorText: expect.any(String),
    }));
    expect(message.parts).toContainEqual(expect.objectContaining({
      type: "text", text: "The tool arguments were invalid.", state: "done",
    }));
  });
});
