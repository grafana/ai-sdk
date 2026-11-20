import { describe, expect, it } from "vitest";
import { fetchScenario } from "./helpers.js";
import { readToolStateStream } from "./ui-tool-state-helpers.js";

describe("Anthropic implicit code execution from March web tools", () => {
  it.each([false, true])("retains dynamic provider output with JSON fallback=%s", async fallback => {
    const { chunks, snapshots } = await readToolStateStream(await fetchScenario(`anthropic-dynamic-web?fallback=${fallback}`));
    for (const type of ["tool-input-start", "tool-input-available"]) {
      expect(chunks.find(chunk => chunk.type === type && "toolCallId" in chunk && chunk.toolCallId === "code-1")).toMatchObject({
        providerExecuted: true, dynamic: true,
      });
    }
    expect(snapshots.at(-1)?.parts.find(part => part.type === "dynamic-tool")).toMatchObject({
      toolName: "code_execution", toolCallId: "code-1", state: "output-available", providerExecuted: true,
    });
  });
});
