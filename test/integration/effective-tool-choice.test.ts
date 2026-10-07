import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import {
  readUIMessageStream,
  uiMessageChunkSchema,
  type UIMessage,
  type UIMessageChunk,
} from "ai";
import { fetchScenario } from "./helpers.js";

async function readScenario(name: string) {
  const response = await fetchScenario(`effective-tool-choice-${name}`);
  expect(response.status).toBe(200);
  const chunks: UIMessageChunk[] = [];
  for await (const parsed of parseJsonEventStream({
    stream: response.body!,
    schema: uiMessageChunkSchema,
  })) {
    expect(parsed.success).toBe(true);
    if (parsed.success) chunks.push(parsed.value);
  }
  const errors: unknown[] = [];
  let message: UIMessage | undefined;
  for await (const next of readUIMessageStream({
    stream: new ReadableStream<UIMessageChunk>({
      start(controller) {
        chunks.forEach((chunk) => controller.enqueue(chunk));
        controller.close();
      },
    }),
    onError: (error) => errors.push(error),
    terminateOnError: false,
  })) {
    message = next;
  }
  return { chunks, errors, message };
}

describe("effective tool choice", () => {
  it("preserves reasoning and text when required choice produces no call", async () => {
    const { chunks, errors, message } = await readScenario("required-miss");
    expect(chunks).toEqual([
      { type: "start", messageId: "message-1" },
      { type: "start-step" },
      { type: "reasoning-start", id: "r1" },
      { type: "reasoning-delta", id: "r1", delta: "thinking" },
      { type: "reasoning-end", id: "r1" },
      { type: "text-start", id: "t1" },
      { type: "text-delta", id: "t1", delta: "response" },
      { type: "text-end", id: "t1" },
      { type: "error", errorText: "tool choice violated" },
      { type: "finish-step" },
      { type: "finish", finishReason: "error" },
    ]);
    expect(errors).toHaveLength(1);
    expect(errors[0]).toHaveProperty("message", "tool choice violated");
    expect(message).toMatchObject({ id: "message-1", role: "assistant" });
    expect(message?.parts).toEqual([
      { type: "step-start" },
      { type: "reasoning", id: "r1", text: "thinking", state: "done" },
      { type: "text", text: "response", state: "done" },
    ]);
  });

  it("keeps an unrelated call visible without new local approval or output", async () => {
    const { chunks, errors, message } = await readScenario("named-miss");
    expect(chunks).toEqual([
      { type: "start", messageId: "message-1" },
      { type: "start-step" },
      { type: "tool-input-available", toolCallId: "call-1", toolName: "other", input: {} },
      { type: "error", errorText: "tool choice violated" },
      { type: "finish-step" },
      { type: "finish", finishReason: "error" },
    ]);
    expect(errors).toHaveLength(1);
    expect(message?.parts).toEqual([
      { type: "step-start" },
      expect.objectContaining({ type: "tool-other", toolCallId: "call-1", input: {}, state: "input-available" }),
    ]);
    expect(message?.parts[1]).not.toHaveProperty("approval");
    expect(message?.parts[1]).toHaveProperty("output", undefined);
  });

  it.each(["required-valid", "named-valid"])("retains normal output for %s", async (name) => {
    const { chunks, errors, message } = await readScenario(name);
    expect(chunks).toEqual([
      { type: "start", messageId: "message-1" },
      { type: "start-step" },
      { type: "tool-input-available", toolCallId: "call-1", toolName: "lookup", input: {} },
      { type: "tool-output-available", toolCallId: "call-1", output: "found" },
      { type: "finish-step" },
      { type: "finish", finishReason: "tool-calls" },
    ]);
    expect(errors).toHaveLength(0);
    expect(message?.parts.find((part) => part.type === "tool-lookup")).toMatchObject({
      toolCallId: "call-1", input: {}, state: "output-available", output: "found",
    });
  });
});
