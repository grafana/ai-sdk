import { randomUUID } from "node:crypto";
import { describe, it, expect } from "vitest";
import { parseJsonEventStream, type ParseResult } from "@ai-sdk/provider-utils";
import { uiMessageChunkSchema, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

// Long enough that a result emitted on completion always arrives, and short
// enough that a result held back until the slow tool finishes fails quickly.
const FAST_OUTPUT_DEADLINE_MS = 5_000;

type ChunkReader = ReadableStreamDefaultReader<ParseResult<UIMessageChunk>>;

async function nextChunk(reader: ChunkReader): Promise<UIMessageChunk | undefined> {
  const { done, value } = await reader.read();
  if (done) return undefined;
  if (!value.success) throw value.error;
  return value.value;
}

function outputFor(chunks: UIMessageChunk[], toolCallId: string): UIMessageChunk | undefined {
  return chunks.find(
    (chunk) =>
      (chunk.type === "tool-output-available" || chunk.type === "tool-output-error") &&
      chunk.toolCallId === toolCallId,
  );
}

describe("concurrent tool execution", () => {
  it("delivers a fast tool's output while a slow tool in the same step is still running", async () => {
    const gate = randomUUID();
    const res = await fetchScenario(`concurrent-tools?gate=${gate}`);
    expect(res.status).toBe(200);

    const reader: ChunkReader = parseJsonEventStream({
      stream: res.body!,
      schema: uiMessageChunkSchema,
    }).getReader();
    let released = false;
    const release = async () => {
      released = true;
      return fetchScenario(`concurrent-tools-release?gate=${gate}`);
    };

    try {
      // The slow tool is called first and cannot finish until the gate opens,
      // so the fast tool's output arrives here only if results are emitted as
      // each tool completes.
      const beforeRelease: UIMessageChunk[] = [];
      let timer: ReturnType<typeof setTimeout> | undefined;
      const deadline = new Promise<"deadline">((resolve) => {
        timer = setTimeout(() => resolve("deadline"), FAST_OUTPUT_DEADLINE_MS);
      });
      try {
        while (!outputFor(beforeRelease, "fast-1")) {
          const next = await Promise.race([nextChunk(reader), deadline]);
          if (next === "deadline") {
            throw new Error("the fast tool's output did not arrive while the slow tool was blocked");
          }
          if (next === undefined) {
            throw new Error("the stream ended before the fast tool's output arrived");
          }
          beforeRelease.push(next);
        }
      } finally {
        clearTimeout(timer);
      }

      expect(outputFor(beforeRelease, "fast-1")).toMatchObject({
        type: "tool-output-available",
        output: { tool: "fast" },
      });
      expect(outputFor(beforeRelease, "slow-1")).toBeUndefined();

      expect((await release()).status).toBe(204);

      const afterRelease: UIMessageChunk[] = [];
      for (let chunk = await nextChunk(reader); chunk; chunk = await nextChunk(reader)) {
        afterRelease.push(chunk);
      }
      expect(outputFor(afterRelease, "slow-1")).toMatchObject({
        type: "tool-output-available",
        output: { tool: "slow" },
      });
      const text = afterRelease.flatMap((chunk) => (chunk.type === "text-delta" ? [chunk.delta] : [])).join("");
      expect(text).toBe("Both tools finished.");
      expect(afterRelease.at(-1)?.type).toBe("finish");
    } finally {
      // A failed assertion must not leave the slow tool, and the request, running.
      if (!released) await release().catch(() => undefined);
      await reader.cancel().catch(() => undefined);
    }
  });
});
