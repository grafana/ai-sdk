import { describe, expect, it } from "vitest";
import { jsonSchema, simulateReadableStream, streamText, tool } from "ai";
import { MockLanguageModelV4 } from "ai/test";

const call = (toolCallId: string, toolName: string, providerExecuted = false) => ({
  type: "tool-call" as const,
  toolCallId,
  toolName,
  input: "{}",
  providerExecuted,
});

const hostedResult = (toolCallId: string, isError = false, preliminary = false) => ({
  type: "tool-result" as const,
  toolCallId,
  toolName: "hosted",
  result: { error: "opaque payload" },
  isError,
  preliminary,
});

describe("pinned step tool-result membership", () => {
  it("includes only final result content, not failures, denials or previews", async () => {
    const inputSchema = jsonSchema({ type: "object" });
    const model = new MockLanguageModelV4({
      doStream: {
        stream: simulateReadableStream({
          chunks: [
            call("ordinary", "ordinary"),
            call("failed", "failed"),
            call("invalid", "missing"),
            call("denied", "denied"),
            call("hosted-ok", "hosted", true),
            call("hosted-error", "hosted", true),
            call("hosted-preview", "hosted", true),
            hostedResult("hosted-preview", false, true),
            hostedResult("hosted-error", true),
            hostedResult("hosted-ok"),
            {
              type: "finish",
              finishReason: { unified: "tool-calls", raw: undefined },
              usage: {
                inputTokens: { total: 2, noCache: 2, cacheRead: undefined, cacheWrite: undefined },
                outputTokens: { total: 1, text: 1, reasoning: undefined },
              },
            },
          ],
        }),
      },
    });
    const ordinary = { error: "ordinary application data" };
    const result = streamText({
      model,
      prompt: "lookup",
      tools: {
        ordinary: tool({ inputSchema, execute: () => ordinary }),
        failed: tool({ inputSchema, execute: async function* () {
          yield "tentative value";
          throw new Error("failed execution");
        } }),
        denied: tool({ inputSchema, execute: () => { throw new Error("must not execute"); } }),
      },
      toolApproval: { denied: { type: "denied", reason: "not authorized" } },
    });

    const full = await Array.fromAsync(result.fullStream);
    const [step] = await result.steps;
    const ids = ["hosted-ok", "ordinary"];
    expect(step.toolResults.map(part => part.toolCallId).sort()).toEqual(ids);
    expect((await result.toolResults).map(part => part.toolCallId).sort()).toEqual(ids);
    expect(step.toolResults.find(part => part.toolCallId === "ordinary")?.output).toEqual(ordinary);
    expect(step.content.flatMap(part => part.type === "tool-error" ? [part.toolCallId] : []).sort()).toEqual([
      "failed", "hosted-error", "invalid",
    ]);
    expect(step.content).toContainEqual(expect.objectContaining({ type: "tool-approval-response", approved: false }));
    expect(full).toContainEqual(expect.objectContaining({ type: "tool-output-denied", toolCallId: "denied" }));
    for (const toolCallId of ["failed", "hosted-preview"]) {
      expect(full).toContainEqual(expect.objectContaining({ type: "tool-result", toolCallId, preliminary: true }));
    }
  });
});
