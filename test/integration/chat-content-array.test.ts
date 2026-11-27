import { expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

it("assembles text and thinking from Chat content arrays in wire order", async () => {
  const response = await fetchScenario("chat-content-array");
  expect(response.status).toBe(200);
  const chunks: UIMessageChunk[] = [];
  for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
    expect(parsed.success).toBe(true);
    if (parsed.success) chunks.push(parsed.value);
  }

  expect(chunks.filter(chunk => chunk.type.startsWith("text-") || chunk.type.startsWith("reasoning-"))).toEqual([
    { type: "text-start", id: "txt-0" },
    { type: "text-delta", id: "txt-0", delta: "First." },
    { type: "text-end", id: "txt-0" },
    { type: "reasoning-start", id: "reasoning-0" },
    { type: "reasoning-delta", id: "reasoning-0", delta: "Check the result." },
    { type: "reasoning-end", id: "reasoning-0" },
    { type: "text-start", id: "second-text" },
    { type: "text-delta", id: "second-text", delta: "Done." },
    { type: "text-end", id: "second-text" },
  ]);
  expect(chunks.some(chunk => chunk.type === "error")).toBe(false);
  expect(chunks.at(-1)).toMatchObject({ type: "finish", finishReason: "stop" });

  let message: UIMessage | undefined;
  for await (const next of readUIMessageStream({
    stream: new ReadableStream<UIMessageChunk>({
      start(controller) {
        chunks.forEach(chunk => controller.enqueue(chunk));
        controller.close();
      },
    }),
    terminateOnError: true,
  })) message = next;
  expect(message?.parts.filter(part => part.type !== "step-start")).toMatchObject([
    { type: "text", text: "First.", state: "done" },
    { type: "reasoning", text: "Check the result.", state: "done" },
    { type: "text", text: "Done.", state: "done" },
  ]);
});
