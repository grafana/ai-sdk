import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { createGateway } from "@ai-sdk/gateway";
import { Output, streamText, tool } from "ai";
import { describe, expect, it } from "vitest";
import { z } from "zod";
import { startRecorder } from "./recorder";

const CONFORMANCE_UI_DIR = resolve(import.meta.dirname, "../../conformance/ui");

const cases = [
  "generated-files/data-and-url",
  "invalid-provider-tool-input",
  "text-metadata-only-delta",
] as const;

describe("ProviderWire V4 provider-independent conformance transport", () => {
  it.each(cases)("preserves the existing UI oracle for %s", async (fixture) => {
    const input = readFileSync(resolve(CONFORMANCE_UI_DIR, fixture, "input.jsonl"), "utf8");
    const expected = readJSONLines(resolve(CONFORMANCE_UI_DIR, fixture, "expected.jsonl"));
    const recorder = await startRecorder([
      { contentType: "text/event-stream", body: renderSSE(input) },
    ]);

    try {
      const tools = fixture === "invalid-provider-tool-input"
        ? { web_search: tool({ inputSchema: z.object({ query: z.string() }) }) }
        : undefined;
      const output = fixture === "text-metadata-only-delta" ? Output.json() : undefined;
      const result = streamText({
        model: createGateway({ apiKey: "conformance-not-a-real-key", baseURL: recorder.baseURL })("capture/model"),
        prompt: "test",
        maxRetries: 0,
        tools,
        output,
      });
      const actual: unknown[] = [];
      for await (const chunk of result.toUIMessageStream({
        generateMessageId: () => "message-1",
      })) {
        actual.push(JSON.parse(JSON.stringify(chunk)) as unknown);
      }
      expect(actual).toEqual(expected);
    } finally {
      await recorder.close();
    }
  });
});

function renderSSE(input: string): string {
  expect(input.endsWith("\n")).toBe(true);
  return input
    .trimEnd()
    .split("\n")
    .map((line) => `data: ${line}\n\n`)
    .join("");
}

function readJSONLines(path: string): unknown[] {
  const input = readFileSync(path, "utf8");
  expect(input.endsWith("\n")).toBe(true);
  return input.trimEnd().split("\n").map((line) => JSON.parse(line) as unknown);
}
