#!/usr/bin/env tsx

import { Buffer } from "node:buffer";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import {
  existsSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  renameSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { createServer, type Server } from "node:http";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { isDeepStrictEqual } from "node:util";
import { fileURLToPath, pathToFileURL } from "node:url";
import { createAmazonBedrock } from "@ai-sdk/amazon-bedrock";
import { createGateway } from "@ai-sdk/gateway";
import { parse as parseYaml } from "yaml";
import {
  buildMessages,
  loadConfig,
  unsupportedGenerateFields,
} from "./common.mts";

const toolDir = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(toolDir, "../../..");
const fixtureDir = resolve(toolDir, "../bedrock/upstream/json-tool-with-answer");
const inputPath = join(fixtureDir, "input.response.json");
const semanticExpectationPath = join(fixtureDir, "expected-generate.json");
export const normalizedArtifactPath = resolve(
  toolDir,
  "../../interop/providerwire-v4/generated/bedrock-json-tool-with-answer.normalized.json",
);

export const sourceFixture = "bedrock/upstream/json-tool-with-answer";
export const sourceFixtureKey = "amazon-bedrock-json-tool-with-answer.1";
export const policyProfile = "providerwire-v4-h2-unary-v1";
export const artifactKind = "derived-policy-projection";
export const generationAuthority =
  "test/conformance/tools/providerwire-v4-unary.mts independent TypeScript projector";
export const checkCommand = "mise run check-providerwire-v4";
export const updateCommand = "mise run update-providerwire-v4-artifacts";

export type UnaryFixtureClassification = "selected" | "gap";

export const unaryFixtureClassifications: Readonly<Record<string, UnaryFixtureClassification>> = {
  [sourceFixture]: "selected",
};

interface RegisteredBaseline {
  commit: string;
  packages: {
    "@ai-sdk/amazon-bedrock": string;
    "@ai-sdk/gateway": string;
    "@ai-sdk/provider": string;
  };
}

export interface UnaryNormalizedArtifact {
  artifactKind: typeof artifactKind;
  source: {
    fixture: typeof sourceFixture;
    upstreamKey: typeof sourceFixtureKey;
    input: "input.response.json";
    inputSha256: string;
  };
  baseline: RegisteredBaseline;
  policyProfile: typeof policyProfile;
  generationAuthority: typeof generationAuthority;
  commands: {
    check: typeof checkCommand;
    update: typeof updateCommand;
  };
  result: Record<string, unknown>;
}

type JSONObject = Record<string, unknown>;

function asJSONObject(value: unknown, name: string): JSONObject {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`${name} must be a JSON object`);
  }
  return value as JSONObject;
}

function serializeJSONObject(value: unknown, name: string): JSONObject {
  const serialized = JSON.stringify(value);
  if (serialized === undefined) {
    throw new Error(`${name} is not JSON serializable`);
  }
  return asJSONObject(JSON.parse(serialized) as unknown, name);
}

function sha256(value: Buffer): string {
  return createHash("sha256").update(value).digest("hex");
}

function findConfigPaths(directory: string): string[] {
  const paths: string[] = [];
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      paths.push(...findConfigPaths(path));
    } else if (entry.isFile() && entry.name === "config.yaml") {
      paths.push(path);
    }
  }
  return paths;
}

export function discoverEligibleUnaryFixtures(
  conformanceRoot = resolve(toolDir, ".."),
): string[] {
  const fixtures: string[] = [];
  for (const configPath of findConfigPaths(conformanceRoot)) {
    const config = asJSONObject(
      parseYaml(readFileSync(configPath, "utf8")),
      `fixture config ${configPath}`,
    );
    if (config.operation !== "generate") continue;

    const directory = dirname(configPath);
    const relative = dirname(configPath.slice(conformanceRoot.length + 1)).replaceAll("\\", "/");
    const segments = relative.split("/");
    if (segments.length < 3 || (segments[1] !== "recorded" && segments[1] !== "upstream")) {
      throw new Error(`unary fixture ${relative} is outside a provider provenance tree`);
    }
    for (const filename of [
      "input.response.json",
      "expected-generate.json",
      "expected-requests.jsonl",
    ]) {
      if (!existsSync(join(directory, filename))) {
        throw new Error(`unary fixture ${relative} is missing ${filename}`);
      }
    }
    if (segments[1] === "upstream") {
      const indexPath = join(conformanceRoot, segments[0], "upstream", "INDEX.yaml");
      if (!existsSync(indexPath)) {
        throw new Error(`unary fixture ${relative} is missing its upstream INDEX.yaml`);
      }
      const index = asJSONObject(
        parseYaml(readFileSync(indexPath, "utf8")),
        `upstream index ${indexPath}`,
      );
      const indexedInput = `${segments.slice(2).join("/")}/input.response.json`;
      if (!Object.values(index).includes(indexedInput)) {
        throw new Error(`unary fixture ${relative} input is not registered in upstream INDEX.yaml`);
      }
    }
    fixtures.push(relative);
  }
  return fixtures.sort();
}

export function assertUnaryFixtureInventory(
  conformanceRoot = resolve(toolDir, ".."),
  classifications: Readonly<Record<string, UnaryFixtureClassification>> = unaryFixtureClassifications,
): string[] {
  const fixtures = discoverEligibleUnaryFixtures(conformanceRoot);
  const unclassified = fixtures.filter(fixture => classifications[fixture] === undefined);
  if (unclassified.length > 0) {
    throw new Error(`unclassified unary fixtures: ${unclassified.join(", ")}`);
  }
  const stale = Object.keys(classifications).filter(fixture => !fixtures.includes(fixture));
  if (stale.length > 0) {
    throw new Error(`classified unary fixtures are no longer eligible: ${stale.join(", ")}`);
  }
  const selected = fixtures.filter(fixture => classifications[fixture] === "selected");
  if (!isDeepStrictEqual(selected, [sourceFixture])) {
    throw new Error(`selected unary fixtures must be exactly ${sourceFixture}`);
  }
  return fixtures;
}

export function loadRegisteredBaseline(): RegisteredBaseline {
  const manifest = asJSONObject(
    parseYaml(readFileSync(resolve(toolDir, "../upstream.yaml"), "utf8")),
    "upstream baseline",
  );
  const upstream = asJSONObject(manifest.upstream, "upstream baseline authority");
  const packages = asJSONObject(manifest.packages, "upstream baseline packages");
  const baseline: RegisteredBaseline = {
    commit: String(upstream.commit),
    packages: {
      "@ai-sdk/amazon-bedrock": String(packages["@ai-sdk/amazon-bedrock"]),
      "@ai-sdk/gateway": String(packages["@ai-sdk/gateway"]),
      "@ai-sdk/provider": String(packages["@ai-sdk/provider"]),
    },
  };
  if (
    baseline.commit !== "d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e" ||
    baseline.packages["@ai-sdk/amazon-bedrock"] !== "5.0.55" ||
    baseline.packages["@ai-sdk/gateway"] !== "4.0.52" ||
    baseline.packages["@ai-sdk/provider"] !== "4.0.7"
  ) {
    throw new Error("ProviderWire V4 unary baseline drifted from the approved authority");
  }
  return baseline;
}

async function listen(server: Server): Promise<number> {
  await new Promise<void>((resolveListen, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      server.off("error", reject);
      resolveListen();
    });
  });
  const address = server.address();
  if (address === null || typeof address === "string") {
    throw new Error("replay server did not bind a TCP address");
  }
  return address.port;
}

function close(server: Server): Promise<void> {
  return new Promise((resolveClose, reject) => {
    server.close(error => (error === undefined ? resolveClose() : reject(error)));
  });
}

async function startSingleResponseServer(
  body: Buffer | string,
): Promise<{ baseURL: string; requests: Buffer[]; close: () => Promise<void> }> {
  const requests: Buffer[] = [];
  let served = false;
  const server = createServer(async (request, response) => {
    const chunks: Buffer[] = [];
    for await (const chunk of request) {
      chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
    }
    requests.push(Buffer.concat(chunks));
    if (served) {
      response.writeHead(500, { "content-type": "text/plain" });
      response.end("fixture already served");
      return;
    }
    served = true;
    response.writeHead(200, {
      "content-type": "application/json",
      date: "Fri, 02 Jan 2026 03:04:05 GMT",
    });
    response.end(body);
  });
  const port = await listen(server);
  return {
    baseURL: `http://127.0.0.1:${port}`,
    requests,
    close: () => close(server),
  };
}

function buildFixturePrompt(): Array<Record<string, unknown>> {
  const config = loadConfig(fixtureDir);
  if (config.operation !== "generate") {
    throw new Error("selected unary fixture must use operation: generate");
  }
  const unsupported = unsupportedGenerateFields(config);
  if (unsupported.length > 0) {
    throw new Error(`selected unary fixture has unsupported fields: ${unsupported.join(", ")}`);
  }
  if (config.messages) {
    return [
      ...(config.system ? [{ role: "system", content: config.system }] : []),
      ...((buildMessages(config, config.prompt ?? "test") ?? []) as Array<Record<string, unknown>>),
    ];
  }
  return [
    ...(config.system ? [{ role: "system", content: config.system }] : []),
    {
      role: "user",
      content: [{ type: "text", text: config.prompt ?? "test" }],
    },
  ];
}

function buildFixtureResponseFormat(): Record<string, unknown> | undefined {
  const config = loadConfig(fixtureDir);
  if (!config.responseFormat) return undefined;
  return {
    type: "json",
    schema: config.responseFormat.schema,
    ...(config.responseFormat.name ? { name: config.responseFormat.name } : {}),
    ...(config.responseFormat.description
      ? { description: config.responseFormat.description }
      : {}),
  };
}

export async function replayRawUnaryResult(): Promise<{
  result: JSONObject;
  inputBefore: Buffer;
  inputAfter: Buffer;
}> {
  loadRegisteredBaseline();
  const config = loadConfig(fixtureDir);
  const inputBefore = readFileSync(inputPath);
  const replay = await startSingleResponseServer(inputBefore);
  try {
    const provider = createAmazonBedrock({
      baseURL: replay.baseURL,
      apiKey: "test-api-key",
      region: "us-east-1",
    });
    const model = provider(config.model);
    const responseFormat = buildFixtureResponseFormat();
    const result = await model.doGenerate({
      prompt: buildFixturePrompt() as never,
      ...(responseFormat ? { responseFormat: responseFormat as never } : {}),
      ...(config.providerOptions
        ? { providerOptions: config.providerOptions as never }
        : {}),
      ...(config.headers ? { headers: config.headers } : {}),
    });
    if (replay.requests.length !== 1) {
      throw new Error(`expected one provider replay request, got ${replay.requests.length}`);
    }
    const serialized = serializeJSONObject(result, "raw LanguageModelV4 generate result");
    if (!Array.isArray(serialized.warnings)) {
      throw new Error("raw LanguageModelV4 generate result is missing warnings array");
    }
    return {
      result: serialized,
      inputBefore,
      inputAfter: readFileSync(inputPath),
    };
  } finally {
    await replay.close();
  }
}

export function projectH2UnaryResult(rawResult: unknown): JSONObject {
  const result = structuredClone(asJSONObject(rawResult, "raw unary result"));
  delete result.providerMetadata;
  delete result.request;

  if (Array.isArray(result.content)) {
    result.content = result.content.map((value, index) => {
      const part = structuredClone(asJSONObject(value, `content/${index}`));
      delete part.providerMetadata;
      return part;
    });
  }

  if (result.usage !== undefined) {
    const usage = structuredClone(asJSONObject(result.usage, "usage"));
    delete usage.raw;
    result.usage = usage;
  }

  if (result.response !== undefined) {
    const response = structuredClone(asJSONObject(result.response, "response"));
    delete response.headers;
    delete response.body;
    delete response.modelId;
    delete response.provider;
    if (Object.keys(response).length === 0) {
      delete result.response;
    } else {
      result.response = response;
    }
  }

  return result;
}

function normalizeLegacySemanticExpectation(value: unknown): JSONObject {
  const expectation = structuredClone(
    asJSONObject(value, "existing expected-generate.json"),
  );
  if (expectation.warnings === undefined) expectation.warnings = [];
  return expectation;
}

function providerSemantics(value: unknown): JSONObject {
  const result = asJSONObject(value, "consumed Gateway unary result");
  const metadata = result.providerMetadata === undefined
    ? undefined
    : asJSONObject(result.providerMetadata, "consumed provider metadata");
  const bedrockMetadata = metadata?.bedrock;
  return {
    content: result.content,
    finishReason: result.finishReason,
    usage: result.usage,
    ...(bedrockMetadata === undefined
      ? {}
      : { providerMetadata: { bedrock: bedrockMetadata } }),
    warnings: result.warnings,
  };
}

function assertSemanticEqual(actual: unknown, expected: unknown, name: string): void {
  if (!isDeepStrictEqual(actual, expected)) {
    throw new Error(
      `${name} mismatch\nexpected: ${JSON.stringify(expected, null, 2)}\nactual: ${JSON.stringify(actual, null, 2)}`,
    );
  }
}

export async function verifyPinnedGatewayConsumption(rawResult: JSONObject): Promise<void> {
  const replay = await startSingleResponseServer(JSON.stringify(rawResult));
  try {
    const model = createGateway({
      apiKey: "providerwire-v4-unary-not-a-real-key",
      baseURL: replay.baseURL,
    })("capture/model");
    const consumed = await model.doGenerate({
      prompt: [{ role: "user", content: [{ type: "text", text: "consume" }] }],
    });
    if (replay.requests.length !== 1) {
      throw new Error(`expected one Gateway replay request, got ${replay.requests.length}`);
    }
    const expected = normalizeLegacySemanticExpectation(
      JSON.parse(readFileSync(semanticExpectationPath, "utf8")) as unknown,
    );
    assertSemanticEqual(
      providerSemantics(serializeJSONObject(consumed, "Gateway result")),
      expected,
      "pinned Gateway consumption",
    );
  } finally {
    await replay.close();
  }
}

export function makeNormalizedArtifact(
  result: JSONObject,
  inputSha256: string,
): UnaryNormalizedArtifact {
  return {
    artifactKind,
    source: {
      fixture: sourceFixture,
      upstreamKey: sourceFixtureKey,
      input: "input.response.json",
      inputSha256,
    },
    baseline: loadRegisteredBaseline(),
    policyProfile,
    generationAuthority,
    commands: { check: checkCommand, update: updateCommand },
    result,
  };
}

export function validateArtifactWithGo(path: string): void {
  execFileSync(
    "go",
    [
      "test",
      "./gateway/providerwire/v4",
      "-run",
      "^TestProviderWireV4UnaryResultArtifact$",
      "-count=1",
    ],
    {
      cwd: repoRoot,
      env: {
        ...process.env,
        GOWORK: "off",
        PROVIDERWIRE_V4_UNARY_RESULT_PATH: path,
      },
      stdio: "pipe",
    },
  );
}

function validateResultWithGo(result: JSONObject): void {
  const directory = mkdtempSync(join(tmpdir(), "providerwire-v4-unary-"));
  const path = join(directory, "result.json");
  try {
    writeFileSync(path, `${JSON.stringify({ result }, null, 2)}\n`);
    validateArtifactWithGo(path);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

export function replaceNormalizedArtifact(
  artifact: UnaryNormalizedArtifact,
  destination = normalizedArtifactPath,
): void {
  const directory = mkdtempSync(join(dirname(destination), ".providerwire-v4-unary-"));
  const staged = join(directory, "normalized.json");
  try {
    writeFileSync(staged, `${JSON.stringify(artifact, null, 2)}\n`);
    validateArtifactWithGo(staged);
    renameSync(staged, destination);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

function readCommittedArtifact(): UnaryNormalizedArtifact {
  return JSON.parse(readFileSync(normalizedArtifactPath, "utf8")) as UnaryNormalizedArtifact;
}

export function assertArtifactMetadata(
  artifact: UnaryNormalizedArtifact,
  expectedInputSha256: string,
): void {
  const expected = makeNormalizedArtifact(artifact.result, expectedInputSha256);
  assertSemanticEqual(
    { ...artifact, result: undefined },
    { ...expected, result: undefined },
    "normalized artifact metadata",
  );
}

export async function buildUnaryEvidence(): Promise<{
  raw: JSONObject;
  normalized: JSONObject;
  artifact: UnaryNormalizedArtifact;
}> {
  assertUnaryFixtureInventory();
  const replayed = await replayRawUnaryResult();
  if (!replayed.inputBefore.equals(replayed.inputAfter)) {
    throw new Error("provider input changed while unary evidence was generated");
  }
  const inputHash = sha256(replayed.inputBefore);
  validateResultWithGo(replayed.result);
  await verifyPinnedGatewayConsumption(replayed.result);
  const normalized = projectH2UnaryResult(replayed.result);
  validateResultWithGo(normalized);
  return {
    raw: replayed.result,
    normalized,
    artifact: makeNormalizedArtifact(normalized, inputHash),
  };
}

export async function runUnaryEvidence(mode: "check" | "update"): Promise<void> {
  const evidence = await buildUnaryEvidence();
  if (mode === "update") {
    replaceNormalizedArtifact(evidence.artifact);
    process.stdout.write("ProviderWire V4 unary normalized projection updated\n");
    return;
  }
  const committed = readCommittedArtifact();
  assertArtifactMetadata(committed, evidence.artifact.source.inputSha256);
  assertSemanticEqual(
    committed.result,
    evidence.normalized,
    "committed normalized unary projection",
  );
  validateArtifactWithGo(normalizedArtifactPath);
  process.stdout.write("ProviderWire V4 unary raw and normalized evidence verified\n");
}

function isMainModule(): boolean {
  const entry = process.argv[1];
  return entry !== undefined && import.meta.url === pathToFileURL(resolve(entry)).href;
}

if (isMainModule()) {
  const mode = process.argv.slice(2);
  if (mode.length !== 1 || (mode[0] !== "--check" && mode[0] !== "--update")) {
    throw new Error("usage: providerwire-v4-unary.mts --check|--update");
  }
  await runUnaryEvidence(mode[0] === "--update" ? "update" : "check");
}
