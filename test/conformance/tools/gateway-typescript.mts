import { createGateway } from "@ai-sdk/gateway";
import { executeScenario } from "./scenario.mts";
import type { TestCase } from "./common.mts";
import { gatewayAPIKey } from "./gateway-runtime.mts";

async function main() {
  const chunks: Buffer[] = [];
  for await (const chunk of process.stdin) chunks.push(Buffer.from(chunk));
  const input = JSON.parse(Buffer.concat(chunks).toString("utf8")) as { testCase: TestCase; baseURL: string; token: string; modelID: string };
  const client = createGateway({ baseURL: input.baseURL, apiKey: gatewayAPIKey, headers: { "X-Access-Token": input.token } });
  const result = await executeScenario(input.testCase, client(input.modelID), AbortSignal.timeout(60_000));
  process.stdout.write(JSON.stringify(result));
}

main().catch(error => { console.error(error); process.exitCode = 1; });
