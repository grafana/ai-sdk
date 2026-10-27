#!/usr/bin/env tsx

import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";
import { parse } from "yaml";
import { createSourceIdNormalizer, loadConfig } from "./common.mts";
import { discoverMatrix, reconcile, runMatrix, summarize, type Client, type Row, type RowResult, type Stage } from "./gateway-matrix.mts";
import { backendKey, command, route, routePrefix, StackError, startStack, type Route, type Stack } from "./gateway-runtime.mts";
import { startReplay } from "./replay.mts";
import { normalizeOpenAIApprovalToolCallIds, type ScenarioResult } from "./scenario.mts";

const tools = dirname(fileURLToPath(import.meta.url));
const root = resolve(tools, "..");
const repository = resolve(root, "../..");
const clientTimeout = 65_000;
const controller = new AbortController();
for (const signal of ["SIGINT", "SIGTERM"] as const) process.once(signal, () => controller.abort(new Error(signal)));

type Replay = Awaited<ReturnType<typeof startReplay>>;

const pluginArchitectures: Partial<Record<NodeJS.Architecture, string>> = { x64: "amd64", arm64: "arm64" };

async function main() {
  const { values } = parseArgs({ options: { "plugin-dir": { type: "string" }, "grafana-image": { type: "string" }, scenario: { type: "string" }, client: { type: "string" }, output: { type: "string" } } });
  if (values.client && values.client !== "go" && values.client !== "typescript") throw new Error("client must be go or typescript");
  if (!values["plugin-dir"] || !values["grafana-image"]) throw new Error("--plugin-dir and --grafana-image are required");
  if (process.platform !== "linux") throw new Error("gateway conformance requires Linux with a local Docker daemon");
  const pluginDirectory = resolve(values["plugin-dir"]);
  const image = values["grafana-image"];
  const output = resolve(values.output ?? join(root, "gateway-results"));
  mkdirSync(output, { recursive: true });
  const secrets = [backendKey];
  const redact = (value: string) => secrets.reduce((text, secret) => text.replaceAll(secret, "<redacted>"), value);
  const save = (path: string, value: unknown) => writeFileSync(join(output, path), redact(JSON.stringify(value, null, 2)) + "\n");
  const report: { scope: string; filter: unknown; metadata: Record<string, unknown>; rows: RowResult[]; errors: string[] } = {
    scope: values.scenario || values.client ? "filtered" : "full", filter: values, metadata: {}, rows: [], errors: [],
  };
  let rows: Row[] = [];
  const replays = new Map<string, Replay>();
  let stack: Stack | undefined;
  const buildDirectory = mkdtempSync(join(tmpdir(), "gateway-conformance-build-"));
  try {
    const matrix = discoverMatrix(root, { scenario: values.scenario, client: values.client as Client | undefined });
    rows = matrix.rows;
    report.rows = rows.map(row => ({ ...row, outcome: "not-executed", stage: "harness", invoked: false, errors: ["infrastructure setup did not finish"] }));
    report.metadata = await runMetadata(pluginDirectory, image);
    save("report.json", report);
    const bridge = (await command("docker", ["network", "inspect", "bridge", "--format", "{{(index .IPAM.Config 0).Gateway}}"])).trim();
    const binary = join(buildDirectory, "gateway-client");
    await command("go", ["build", "-mod=readonly", "-tags", "conformance", "-o", binary, "./cmd/gateway-client"], { cwd: root, timeout: 180_000, signal: controller.signal });
    const routes = new Map<string, Route>();
    const unrouted = new Map<string, string>();
    for (const [index, row] of rows.entries()) {
      try {
        const replay = await startReplay(row, bridge);
        replays.set(row.id, replay);
        routes.set(row.id, route(index, row.id, row.provider, loadConfig(row.dir).model, Number(new URL(replay.url).port)));
      } catch (error) {
        unrouted.set(row.id, String(error));
      }
    }
    stack = await startStack({
      image, pluginDirectory, routes: [...routes.values()], signal: controller.signal,
      settingsSource: readFileSync(join(repository, "ai-gateway/provisioning/grafana.ini"), "utf8"),
      provisioningSource: readFileSync(join(repository, "ai-gateway/provisioning/plugins/apps.yaml"), "utf8"),
    });
    secrets.push(stack.token, stack.adminPassword);
    report.metadata.boots = stack.boots;
    report.metadata.rejectedProviders = Object.fromEntries(stack.rejected);
    const admitted = new Set(stack.admitted.map(r => r.rowID));
    let stopped: string | undefined;
    let index = 0;
    const attempt = async (row: Row): Promise<RowResult> => {
      if (stopped) return { ...row, outcome: "not-executed", stage: "harness", invoked: false, errors: [stopped] };
      const result: RowResult = { ...row, outcome: "failed", stage: "harness", invoked: false, errors: [], artifact: `row-${index++}.json` };
      const replay = replays.get(row.id);
      const target = routes.get(row.id);
      const evidence: Record<string, unknown> = { route: target };
      let capture: ScenarioResult = { chunks: [], usage: [], object: null };
      let stage: Stage = "harness";
      const started = new Date();
      try {
        if (!replay || !target) throw new Error(unrouted.get(row.id) ?? "row has no replay route");
        const rejection = stack!.rejected.get(row.provider);
        if (rejection) {
          stage = "provider-setup";
          evidence.startupLogs = rejection.evidence;
          throw new Error(`gateway rejected the ${row.provider} provider configuration at startup; client not invoked`);
        }
        if (!admitted.has(row.id)) throw new Error("row is missing from the admitted gateway catalog");
        stage = "execution";
        result.invoked = true;
        const input = { operation: "execute", directory: row.dir, testCase: { name: row.name, dir: row.dir, provider: row.provider }, baseURL: `${stack!.url}${routePrefix}`, token: stack!.token, modelID: target.modelID };
        const captured = row.client === "go"
          ? await command(binary, [], { input, timeout: clientTimeout, signal: controller.signal })
          : await command(process.execPath, ["--import", "tsx", join(tools, "gateway-typescript.mts")], { cwd: tools, input, timeout: clientTimeout, signal: controller.signal });
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
          const errors = JSON.parse(await command(binary, [], { input: { operation: "compare", directory: row.dir, result: capture, requests: replay?.requests ?? [], gateway: true } })) as string[];
          result.errors.push(...errors);
        } catch (error) {
          stage = "harness";
          result.errors.push(`comparison failed: ${error}`);
        }
        if (replay?.errors.length) {
          stage = "harness";
          result.errors.push(...replay.errors);
        }
        if (result.invoked) {
          try { evidence.gatewayLogs = await stack!.logs(started, new Date()); }
          catch (error) { result.errors.push(`collecting gateway logs: ${error}`); stage = "harness"; }
          if (result.errors.length && !stopped && !(await stack!.alive())) stopped = `gateway stack stopped while running ${row.id}`;
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
    if (error instanceof StackError) save("stack-startup.json", { error: error.message, ...error.evidence });
  } finally {
    if (stack) {
      try { await stack.close(); }
      catch (error) { report.errors.push(`gateway stack cleanup: ${error}`); }
    }
    for (const replay of replays.values()) {
      try { await replay.close(); }
      catch (error) { report.errors.push(`replay cleanup: ${error}`); }
    }
    rmSync(buildDirectory, { recursive: true, force: true });
    const present = new Set(report.rows.map(row => row.id));
    for (const row of rows) if (!present.has(row.id)) report.rows.push({ ...row, outcome: "not-executed", invoked: false, stage: "harness", errors: ["runner did not complete"] });
    report.errors.push(...reconcile(rows, report.rows));
    save("report.json", report);
    writeFileSync(join(output, "summary.md"), redact(summarize(report.rows) + (report.errors.length ? `\n## Harness errors\n\n${report.errors.join("\n")}\n` : "")));
    console.log(`Report: ${join(output, "report.json")}`);
  }
  if (report.errors.length || report.rows.some(row => row.outcome !== "passed")) process.exitCode = 1;
}

async function runMetadata(pluginDirectory: string, image: string): Promise<Record<string, unknown>> {
  const architecture = pluginArchitectures[process.arch];
  if (!architecture) throw new Error(`unsupported host architecture: ${process.arch}`);
  const backend = join(pluginDirectory, `gpx_aigateway_linux_${architecture}`);
  const revision = join(pluginDirectory, "SOURCE_REVISION");
  const imageInfo = JSON.parse(await command("docker", ["image", "inspect", image]).catch(async () => {
    await command("docker", ["pull", image], { timeout: 600_000, signal: controller.signal });
    return command("docker", ["image", "inspect", image]);
  }))[0];
  return {
    baseline: parse(readFileSync(join(root, "upstream.yaml"), "utf8")),
    clientPackages: JSON.parse(readFileSync(join(tools, "package.json"), "utf8")).dependencies,
    sourceRevision: (await command("git", ["rev-parse", "HEAD"], { cwd: repository })).trim(),
    checkoutStatus: await command("git", ["status", "--short"], { cwd: repository }),
    goVersion: (await command("go", ["version"])).trim(),
    grafanaImage: { requested: image, id: imageInfo.Id, digests: imageInfo.RepoDigests },
    plugin: {
      directory: pluginDirectory,
      sourceRevision: existsSync(revision) ? readFileSync(revision, "utf8").trim() : undefined,
      backendModules: await command("go", ["version", "-m", backend]),
    },
  };
}

main().catch(error => { console.error(error); process.exitCode = 1; });
