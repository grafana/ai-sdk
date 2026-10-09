import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { readFileSync, writeFileSync } from "node:fs";
import { join, resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { createAnthropic } from "@ai-sdk/anthropic";

const CASES_DIR = resolve(dirname(fileURLToPath(import.meta.url)), "../testdata/anthropic-unary");
const MODEL_ID = "claude-fable-5-1";

type SyntheticCase = { name: string; description: string; response: Record<string, unknown> };

async function withReplayServer<T>(body: string, run: (port: number) => Promise<T>): Promise<T> {
  const server = createServer((req: IncomingMessage, res: ServerResponse) => {
    req.resume();
    req.on("end", () => {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(body);
    });
  });
  const port = await new Promise<number>(resolvePort => {
    server.listen(0, "127.0.0.1", () => resolvePort((server.address() as { port: number }).port));
  });
  try {
    return await run(port);
  } finally {
    await new Promise<void>(done => server.close(() => done()));
  }
}

async function generateCase(tc: SyntheticCase): Promise<Record<string, unknown>> {
  return withReplayServer(JSON.stringify(tc.response), async port => {
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
