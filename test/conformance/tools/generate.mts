#!/usr/bin/env tsx

import { existsSync, writeFileSync } from "node:fs";
import { join, resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { createAnthropic } from "@ai-sdk/anthropic";
import { createOpenAI } from "@ai-sdk/openai";
import { createAmazonBedrock } from "@ai-sdk/amazon-bedrock";
import { createOpenAICompatible } from "@ai-sdk/openai-compatible";
import { loadConfig, writeRequestSnapshots, type TestCase } from "./common.mts";
import { discoverCases } from "./gateway-matrix.mts";
import { startReplay } from "./replay.mts";
import { executeScenario } from "./scenario.mts";

const CONFORMANCE_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");

export function createModel(providerName: string, modelID: string, baseURL: string) {
  switch (providerName) {
    case "anthropic":
      return createAnthropic({ baseURL: `${baseURL}/v1`, apiKey: "test-api-key" })(modelID);
    case "openai":
      return createOpenAI({ baseURL: `${baseURL}/v1`, apiKey: "test-api-key" }).responses(modelID);
    case "bedrock":
      return createAmazonBedrock({ baseURL, apiKey: "test-api-key", region: "us-east-1" })(modelID);
    case "openai-compatible":
      return createOpenAICompatible({ name: "openai-compatible", baseURL: `${baseURL}/v1`, apiKey: "test-api-key", includeUsage: true, supportsStructuredOutputs: true })(modelID);
    default:
      throw new Error(`Unknown provider: ${providerName}`);
  }
}

async function generateExpected(tc: TestCase) {
  const cfg = loadConfig(tc.dir);
  const replay = await startReplay(tc);
  try {
    const result = await executeScenario(tc, createModel(tc.provider, cfg.model, replay.url));
    if (cfg.operation === "generate") {
      if (result.error) throw new Error(result.error);
      writeFileSync(join(tc.dir, "expected-generate.json"), JSON.stringify(result.generate, null, 2) + "\n");
    } else {
      writeFileSync(join(tc.dir, "expected.jsonl"), result.chunks.map(chunk => JSON.stringify(chunk)).join("\n") + "\n");
      if (existsSync(join(tc.dir, "expected-usage.json"))) writeFileSync(join(tc.dir, "expected-usage.json"), JSON.stringify(result.usage, null, 2) + "\n");
      if (result.outputError) console.log(`  output validation failed: ${result.outputError}`);
      else if (cfg.responseFormat) writeFileSync(join(tc.dir, "expected-object.json"), JSON.stringify(result.object, null, 2) + "\n");
    }
    if (replay.errors.length) throw new Error(replay.errors.join("\n"));
    writeRequestSnapshots(join(tc.dir, "expected-requests.jsonl"), replay.requests);
    console.log(`  OK: ${result.chunks.length} chunks, ${replay.requests.length} request(s)`);
  } finally {
    await replay.close();
  }
}

async function main() {
  const index = process.argv.indexOf("--scenario");
  const filter = index === -1 ? undefined : process.argv[index + 1];
  const cases = discoverCases(CONFORMANCE_ROOT).filter(tc => !filter || tc.name.includes(filter));
  if (!cases.length) throw new Error("No test cases found");
  let errors = 0;
  for (const tc of cases) {
    console.log(`Generating: ${tc.name}`);
    try { await generateExpected(tc); }
    catch (error) { console.error(error); errors++; }
  }
  if (errors) throw new Error(`${errors} error(s) encountered`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch(error => { console.error(error); process.exitCode = 1; });
}
