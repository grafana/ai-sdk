#!/usr/bin/env tsx

import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";
import { parse } from "yaml";
import { createSourceIdNormalizer, loadConfig } from "./common.mts";
import { discoverMatrix, reconcile, runMatrix, summarize, type Client, type Row, type RowResult, type Stage } from "./gateway-matrix.mts";
import { authentication, backendKey, command, createNetwork, gatewayAPIKey, gatewayConfig, modelID, removeResource, startAuthentication, startGateway } from "./gateway-runtime.mts";
import { startReplay } from "./replay.mts";
import { normalizeOpenAIApprovalToolCallIds, type ScenarioResult } from "./scenario.mts";

const tools = dirname(fileURLToPath(import.meta.url));
const root = resolve(tools, "..");
const repository = resolve(root, "../..");
const controller = new AbortController();
for (const signal of ["SIGINT", "SIGTERM"] as const) process.once(signal, () => controller.abort(new Error(signal)));

async function main() {
  const { values } = parseArgs({ options: { image: { type: "string" }, scenario: { type: "string" }, client: { type: "string" }, output: { type: "string" } } });
  if (values.client && values.client !== "go" && values.client !== "typescript") throw new Error("client must be go or typescript");
  const output = resolve(values.output ?? join(root, "gateway-results"));
  mkdirSync(output, { recursive: true });
  const authMaterial = authentication();
  const token = authMaterial.token;
  const redact = (value: string) => [token, backendKey, gatewayAPIKey].reduce((text, secret) => text.replaceAll(secret, "<redacted>"), value);
  const save = (path: string, value: unknown) => writeFileSync(join(output, path), redact(JSON.stringify(value, null, 2)) + "\n");
  const report: { scope: string; filter: unknown; metadata: Record<string, unknown>; rows: RowResult[]; errors: string[] } = {
    scope: values.scenario || values.client ? "filtered" : "full", filter: values, metadata: {}, rows: [], errors: [],
  };
  let rows: Row[] = [];
  let network: Awaited<ReturnType<typeof createNetwork>> | undefined;
  let buildDirectory: string | undefined;
  let auth: Awaited<ReturnType<typeof startAuthentication>> | undefined;
  try {
    const matrix = discoverMatrix(root, { scenario: values.scenario, client: values.client as Client | undefined });
    rows = matrix.rows;
    report.rows = rows.map(row => ({ ...row, outcome: "not-executed", stage: "harness", invoked: false, errors: ["infrastructure setup did not finish"] }));
    report.metadata.baseline = parse(readFileSync(join(root, "upstream.yaml"), "utf8"));
    report.metadata.clientPackages = JSON.parse(readFileSync(join(tools, "package.json"), "utf8")).dependencies;
    report.metadata.checkoutGatewayGoMod = readFileSync(join(repository, "ai-gateway/go.mod"), "utf8");
    report.metadata.goVersion = (await command("go", ["version"])).trim();
    report.metadata.sourceRevision = (await command("git", ["rev-parse", "HEAD"], { cwd: repository })).trim();
    report.metadata.checkoutStatus = await command("git", ["status", "--short"], { cwd: repository });
    save("report.json", report);
    const image = values.image ?? "ai-gateway:conformance";
    if (!values.image) await command("docker", ["build", "--build-arg", `VCS_REF=${report.metadata.sourceRevision}`, "--tag", image, "ai-gateway"], { cwd: repository, timeout: 600_000, signal: controller.signal });
    const imageInfo = JSON.parse(await command("docker", ["image", "inspect", image]))[0];
    report.metadata.image = { requested: image, id: imageInfo.Id, labels: imageInfo.Config.Labels };
    const modulesContainer = `conformance-modules-${process.pid}-${Date.now()}`;
    try {
      report.metadata.imageModules = await command("docker", ["run", "--name", modulesContainer, "--network", "none", "--entrypoint", "/bin/cat", imageInfo.Id, "/usr/share/licenses/grafana-ai-gateway/THIRD_PARTY_MODULES.txt"]);
    } finally {
      await removeResource("container", modulesContainer);
    }
    buildDirectory = mkdtempSync(join(tmpdir(), "gateway-conformance-build-"));
    const binary = join(buildDirectory, "gateway-client");
    await command("go", ["build", "-mod=readonly", "-tags", "conformance", "-o", binary, "./cmd/gateway-client"], { cwd: root, timeout: 180_000, signal: controller.signal });
    network = await createNetwork();
    auth = await startAuthentication(network.host, authMaterial.jwks);
    let index = 0;
    const attempt = async (row: Row): Promise<RowResult> => {
      const result: RowResult = { ...row, outcome: "failed", stage: "harness", invoked: false, errors: [], artifact: `row-${index++}.json` };
      const evidence: Record<string, unknown> = {};
      let replay: Awaited<ReturnType<typeof startReplay>> | undefined;
      let gateway: Awaited<ReturnType<typeof startGateway>> | undefined;
      let capture: ScenarioResult = { chunks: [], usage: [], object: null };
      let stage: Stage = "harness";
      try {
        const cfg = loadConfig(row.dir);
        replay = await startReplay(row, network!.host);
        const configuration = gatewayConfig(row, cfg, replay.url);
        evidence.configuration = configuration;
        gateway = await startGateway(imageInfo.Id, network!.name, configuration, auth!.url, controller.signal);
        if (gateway.setupError) {
          stage = gateway.failureStage;
          evidence.startupState = gateway.startupState;
          evidence.gatewayLogs = gateway.startupLogs;
          throw new Error(gateway.setupError);
        }
        stage = "execution";
        result.invoked = true;
        const baseURL = `${gateway.url}/api/v1/aisdk`;
        const input = { operation: "execute", directory: row.dir, testCase: row, baseURL, token, modelID };
        const captured = row.client === "go"
          ? await command(binary, [], { input, timeout: 65_000, signal: controller.signal })
          : await command(process.execPath, ["--import", "tsx", join(tools, "gateway-typescript.mts")], { cwd: tools, input, timeout: 65_000, signal: controller.signal });
        stage = "harness";
        capture = JSON.parse(captured) as ScenarioResult;
        if (!Array.isArray(capture.chunks) || !Array.isArray(capture.usage)) throw new Error("invalid client capture");
        if (row.client === "go" && row.provider === "openai") {
          capture.chunks = normalizeOpenAIApprovalToolCallIds(row.provider, capture.chunks.map(createSourceIdNormalizer()));
        }
        stage = "comparison";
      } catch (error) {
        result.errors.push(String(error));
      } finally {
        evidence.capture = capture;
        evidence.requests = replay?.requests ?? [];
        try {
          const errors = JSON.parse(await command(binary, [], { input: { operation: "compare", directory: row.dir, result: capture, requests: replay?.requests ?? [] } })) as string[];
          result.errors.push(...errors);
        } catch (error) {
          stage = "harness";
          result.errors.push(`comparison failed: ${error}`);
        }
        if (replay?.errors.length) {
          stage = "harness";
          result.errors.push(...replay.errors);
        }
        if (gateway) {
          try { evidence.gatewayLogs ??= await gateway.logs(); }
          catch (error) { result.errors.push(`collecting gateway logs: ${error}`); stage = "harness"; }
          try { await gateway.close(); }
          catch (error) { result.errors.push(`gateway cleanup: ${error}`); stage = "harness"; }
        }
        if (replay) {
          try { await replay.close(); }
          catch (error) { result.errors.push(`replay cleanup: ${error}`); stage = "harness"; }
        }
      }
      result.stage = stage;
      result.outcome = result.errors.length ? "failed" : "passed";
      save(result.artifact!, { result, ...evidence });
      return result;
    };
    report.rows = [];
    await runMatrix(rows, attempt, controller.signal, row => {
      report.rows.push(row);
      console.log(`${row.outcome}: ${row.id} (${row.stage})`);
      save("report.json", report);
    });
  } catch (error) {
    report.errors.push(String(error));
  } finally {
    if (auth) {
      try { await auth.close(); }
      catch (error) { report.errors.push(`authentication cleanup: ${error}`); }
    }
    if (network) {
      try { await network.close(); }
      catch (error) { report.errors.push(`network cleanup: ${error}`); }
    }
    if (buildDirectory) rmSync(buildDirectory, { recursive: true, force: true });
    const present = new Set(report.rows.map(row => row.id));
    for (const row of rows) if (!present.has(row.id)) report.rows.push({ ...row, outcome: "not-executed", invoked: false, stage: "harness", errors: ["runner did not complete"] });
    report.errors.push(...reconcile(rows, report.rows));
    save("report.json", report);
    writeFileSync(join(output, "summary.md"), redact(summarize(report.rows) + (report.errors.length ? `\n## Harness errors\n\n${report.errors.join("\n")}\n` : "")));
    console.log(`Report: ${join(output, "report.json")}`);
  }
  if (report.errors.length || report.rows.some(row => row.outcome !== "passed")) process.exitCode = 1;
}

main().catch(error => { console.error(error); process.exitCode = 1; });
