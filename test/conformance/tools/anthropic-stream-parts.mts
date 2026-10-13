import { readFileSync, writeFileSync } from "node:fs";
import { join, resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { createAnthropic } from "@ai-sdk/anthropic";
import { withReplayServer } from "./replay-server.mts";
import { normalizeCall } from "./provider-parts.mts";

const CASES_DIR = resolve(dirname(fileURLToPath(import.meta.url)), "../testdata/anthropic-stream-parts");
const MODEL_ID = "claude-sonnet-4-5";

type SyntheticCase = { name: string; description: string; events: Record<string, unknown>[] };

function toSSE(events: Record<string, unknown>[]): string {
  return events.map(event => `event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`).join("");
}

async function generateCase(tc: SyntheticCase): Promise<Record<string, unknown>> {
  return withReplayServer(toSSE(tc.events), { "Content-Type": "text/event-stream", "Cache-Control": "no-cache" }, async port => {
    const model = createAnthropic({ baseURL: `http://127.0.0.1:${port}/v1`, apiKey: "test-api-key" })(MODEL_ID) as any;
    try {
      const result = await model.doStream({
        prompt: [{ role: "user", content: [{ type: "text", text: "test" }] }],
        includeRawChunks: true,
      });
      const parts: Record<string, unknown>[] = [];
      for await (const part of result.stream as any) parts.push(part);
      return { name: tc.name, parts: normalizeCall(parts, "anthropic") };
    } catch {
      return { name: tc.name, callError: true };
    }
  });
}

export async function generateAnthropicStreamParts(): Promise<void> {
  const cases: SyntheticCase[] = JSON.parse(readFileSync(join(CASES_DIR, "cases.json"), "utf8"));
  const lines: string[] = [];
  for (const tc of cases) {
    console.log(`Generating: anthropic-stream-parts/${tc.name}`);
    lines.push(JSON.stringify(await generateCase(tc)));
  }
  writeFileSync(join(CASES_DIR, "expected-parts.jsonl"), lines.join("\n") + "\n");
}
