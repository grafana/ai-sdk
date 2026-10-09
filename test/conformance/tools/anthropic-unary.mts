import { readFileSync, writeFileSync } from "node:fs";
import { join, resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { createAnthropic } from "@ai-sdk/anthropic";
import { withReplayServer } from "./replay-server.mts";

const CASES_DIR = resolve(dirname(fileURLToPath(import.meta.url)), "../testdata/anthropic-unary");
const MODEL_ID = "claude-fable-5-1";

type SyntheticCase = { name: string; description: string; response: Record<string, unknown> };

async function generateCase(tc: SyntheticCase): Promise<Record<string, unknown>> {
  return withReplayServer(JSON.stringify(tc.response), { "Content-Type": "application/json" }, async port => {
    const model = createAnthropic({ baseURL: `http://127.0.0.1:${port}/v1`, apiKey: "test-api-key" })(MODEL_ID) as any;
    try {
      const result = await model.doGenerate({
        prompt: [{ role: "user", content: [{ type: "text", text: "test" }] }],
      });
      const projected = {
        content: result.content,
        finishReason: result.finishReason,
        usage: result.usage,
        providerMetadata: result.providerMetadata,
        ...(result.warnings?.length ? { warnings: result.warnings } : {}),
      };
      return { name: tc.name, result: JSON.parse(JSON.stringify(projected)) };
    } catch {
      return { name: tc.name, callError: true };
    }
  });
}

export async function generateAnthropicUnary(): Promise<void> {
  const cases: SyntheticCase[] = JSON.parse(readFileSync(join(CASES_DIR, "cases.json"), "utf8"));
  const lines: string[] = [];
  for (const tc of cases) {
    console.log(`Generating: anthropic-unary/${tc.name}`);
    lines.push(JSON.stringify(await generateCase(tc)));
  }
  writeFileSync(join(CASES_DIR, "expected-results.jsonl"), lines.join("\n") + "\n");
}
