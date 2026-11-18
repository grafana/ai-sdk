import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("stream startup", () => {
  it.each([
    ["stream-start-only", { type: "error", errorText: "An error occurred." }],
    ["prepare-step-abort", { type: "abort", reason: "startup canceled" }],
    ["do-stream-abort", { type: "abort", reason: "startup canceled" }],
  ] as const)("parses %s without an empty step", async (name, terminal) => {
    const response = await fetchScenario(name);
    expect(response.status).toBe(200);
    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      if (!parsed.success) throw parsed.error;
      chunks.push(parsed.value);
    }
    expect(chunks).toEqual([{ type: "start" }, terminal]);

    const errors: unknown[] = [];
    for await (const message of readUIMessageStream({
      stream: new ReadableStream<UIMessageChunk>({
        start(controller) {
          for (const chunk of chunks) controller.enqueue(chunk);
          controller.close();
        },
      }),
      onError: error => errors.push(error),
    })) expect(message.parts).toEqual([]);
    if (terminal.type === "error") {
      expect(errors).toMatchObject([{ message: terminal.errorText }]);
    } else {
      expect(errors).toEqual([]);
    }
  });
});
