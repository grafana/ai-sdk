import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, simulateReadableStream, tool, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { MockLanguageModelV4 } from "ai/test";
import { z } from "zod";
import { expect } from "vitest";

export function json<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

type Part = Record<string, unknown> & { type: string };

export function canonicalUI(messages: unknown): unknown {
  const result = json(messages) as Array<{ parts: Part[] }>;
  for (const message of result) {
    for (const part of message.parts) {
      if (!part.type.startsWith("tool-") && part.type !== "dynamic-tool") continue;
      if (part.type.startsWith("tool-")) delete part.toolName;
      if (part.providerExecuted === false) delete part.providerExecuted;
      if (part.approval != null) {
        const approval = part.approval as Record<string, unknown>;
        if (approval.isAutomatic === false) delete approval.isAutomatic;
        if (approval.signature === "") delete approval.signature;
      }
    }
  }
  return result;
}

export function canonicalModel(messages: unknown): unknown {
  const result = json(messages) as Array<{ content: string | Part[] }>;
  for (const message of result) {
    if (!Array.isArray(message.content)) continue;
    for (const part of message.content) {
      if (part.providerOptions != null && Object.keys(part.providerOptions as object).length === 0) delete part.providerOptions;
      if (part.type === "tool-call" || part.type === "tool-approval-response") {
        if (part.providerExecuted === false) delete part.providerExecuted;
      }
      if (part.type === "tool-approval-request" || part.type === "tool-approval-response") {
        if (part.reason === "") delete part.reason;
      }
      if (part.type === "tool-approval-request") {
        if (part.isAutomatic === false) delete part.isAutomatic;
        if (part.signature === "") delete part.signature;
      }
      const output = part.output as Record<string, unknown> | undefined;
      if (output?.type === "execution-denied" && output.reason === "") delete output.reason;
    }
  }
  return result;
}

export const toolStateReferenceTools = { lookup: tool({ inputSchema: z.object({ q: z.string() }), outputSchema: z.string(), execute: async () => "approved" }) };

export function referenceToolStateModel(finalizePreliminary = false) {
  return new MockLanguageModelV4({ doStream: {
    stream: simulateReadableStream({ chunks: [
      ...(finalizePreliminary ? [{ type: "tool-result" as const, toolCallId: "pre", toolName: "lookup", result: "final" }] : []),
      { type: "text-start", id: "t" }, { type: "text-delta", id: "t", delta: "resumed" }, { type: "text-end", id: "t" },
      { type: "finish", finishReason: { unified: "stop", raw: undefined }, usage: { inputTokens: { total: 0, noCache: 0, cacheRead: 0, cacheWrite: 0 }, outputTokens: { total: 0, text: 0, reasoning: 0 } } },
    ] }),
  } });
}

export async function readToolStateStream(response: Response, message?: UIMessage): Promise<{ chunks: UIMessageChunk[]; snapshots: UIMessage[] }> {
  expect(response.ok).toBe(true);
  const chunks: UIMessageChunk[] = [];
  const stream = parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema }).pipeThrough(new TransformStream({
    transform(result, controller) {
      expect(result.success).toBe(true);
      if (result.success) { chunks.push(json(result.value)); controller.enqueue(result.value); }
    },
  }));
  const snapshots: UIMessage[] = [];
  for await (const snapshot of readUIMessageStream({ stream, message, terminateOnError: true })) snapshots.push(json(snapshot));
  return { chunks, snapshots };
}
