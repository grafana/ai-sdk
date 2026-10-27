import { readFileSync, writeFileSync } from "node:fs";
import { join, resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { createAnthropic } from "@ai-sdk/anthropic";
import { normalizeRequestSnapshot, type RequestSnapshot } from "./common.mts";
import { withReplayServer } from "./replay-server.mts";

const CASES_DIR = resolve(dirname(fileURLToPath(import.meta.url)), "../testdata/anthropic-prompt-conversion");
const MODEL_ID = "claude-sonnet-4-5";
const MAX_OUTPUT_TOKENS = 1024;
const RESPONSE = {
  id: "msg_1",
  type: "message",
  role: "assistant",
  model: MODEL_ID,
  content: [{ type: "text", text: "ok" }],
  stop_reason: "end_turn",
  stop_sequence: null,
  usage: { input_tokens: 1, output_tokens: 1 },
};

type SyntheticCase = { name: string; description: string; prompt: unknown[] };

async function generateCase(tc: SyntheticCase): Promise<Record<string, unknown>> {
  let request: RequestSnapshot | undefined;
  return withReplayServer(
    JSON.stringify(RESPONSE),
    { "Content-Type": "application/json" },
    async port => {
      const model = createAnthropic({ baseURL: `http://127.0.0.1:${port}/v1`, apiKey: "test-api-key" })(MODEL_ID) as any;
      const result = await model.doGenerate({ prompt: tc.prompt, maxOutputTokens: MAX_OUTPUT_TOKENS });
      if (!request) throw new Error(`no request captured for ${tc.name}`);
      return { name: tc.name, request, warnings: result.warnings };
    },
    (req, body) => {
      request = normalizeRequestSnapshot("anthropic", req, body);
    },
  );
}

export async function generateAnthropicPromptConversion(): Promise<void> {
  const cases: SyntheticCase[] = JSON.parse(readFileSync(join(CASES_DIR, "cases.json"), "utf8"));
  const lines: string[] = [];
  for (const tc of cases) {
    console.log(`Generating: anthropic-prompt-conversion/${tc.name}`);
    lines.push(JSON.stringify(await generateCase(tc)));
  }
  writeFileSync(join(CASES_DIR, "expected-results.jsonl"), lines.join("\n") + "\n");
}
