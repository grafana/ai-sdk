import { spawn } from "node:child_process";
import { chmodSync, cpSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { randomUUID } from "node:crypto";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { stringify } from "yaml";
import { replayAdapter } from "./replay.mts";

const commandTimeout = 30_000;
const maxCommandOutput = 4 * 1024 * 1024;
const maxLogBytes = 64 * 1024;
const startupTimeout = 180_000;
const catalogTimeout = 60_000;
const pollInterval = 500;
const containerPort = "3000/tcp";
const pluginSection = "plugin.grafana-aigateway-app";
export const pluginID = "grafana-aigateway-app";
export const routePrefix = "/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/default";
export const containerHost = "host.docker.internal";
export const backendKey = "conformance-backend-key";
const providerKeySetting = "conformance_provider_key";
const providerKeyEnv = `GF_PLUGIN_${providerKeySetting.toUpperCase()}`;
const pluginMount = `/var/lib/grafana/plugins/${pluginID}`;
const settingsMount = "/etc/grafana/conformance.ini";
const catalogMount = "/etc/grafana/aigateway";
const provisioningMount = "/etc/grafana/provisioning";

export type Run = typeof command;

export function command(file: string, args: string[], options: { cwd?: string; input?: unknown; timeout?: number; signal?: AbortSignal; includeStderr?: boolean } = {}): Promise<string> {
  return new Promise((resolve, reject) => {
    const env = Object.fromEntries(["PATH", "HOME", "TMPDIR", "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_CONFIG", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH", "GOTOOLCHAIN", "GOCACHE", "GOMODCACHE"].flatMap(key => process.env[key] ? [[key, process.env[key]!]] : []));
    const child = spawn(file, args, { cwd: options.cwd, detached: process.platform !== "win32", env: { ...env, GOWORK: "off" }, stdio: ["pipe", "pipe", "pipe"] });
    let stdout = "";
    let stderr = "";
    let failure: Error | undefined;
    const kill = (error: Error) => {
      failure ??= error;
      if (child.pid) {
        try {
          if (process.platform === "win32") child.kill("SIGKILL");
          else process.kill(-child.pid, "SIGKILL");
        } catch (error) {
          if ((error as NodeJS.ErrnoException).code !== "ESRCH") failure = new Error(`process cleanup failed: ${error}`);
        }
      }
    };
    const abort = () => kill(new Error("command canceled"));
    const timer = setTimeout(() => kill(new Error(`command timed out: ${file}`)), options.timeout ?? commandTimeout);
    options.signal?.addEventListener("abort", abort, { once: true });
    if (options.signal?.aborted) abort();
    child.stdout.on("data", data => {
      stdout += data.toString();
      if (Buffer.byteLength(stdout) > maxCommandOutput) kill(new Error("command stdout exceeded limit"));
    });
    child.stderr.on("data", data => {
      stderr += data.toString();
      if (Buffer.byteLength(stderr) > maxCommandOutput) kill(new Error("command stderr exceeded limit"));
    });
    child.on("error", error => { failure = error; });
    child.stdin.on("error", error => { failure ??= error; });
    child.on("close", code => {
      clearTimeout(timer);
      options.signal?.removeEventListener("abort", abort);
      if (failure || code !== 0) reject(new Error(`${failure?.message ?? `${file} exited ${code}`}\n${stderr.slice(-65536)}`));
      else resolve(stdout + (options.includeStderr ? stderr : ""));
    });
    child.stdin.end(options.input === undefined ? undefined : JSON.stringify(options.input));
  });
}

export interface Route {
  rowID: string;
  instance: string;
  modelID: string;
  provider: string;
  model: string;
  replayURL: string;
}

export function route(index: number, rowID: string, provider: string, model: string, replayPort: number): Route {
  return {
    rowID, provider, model,
    instance: `row-${index}`,
    modelID: `conformance/row-${index}`,
    replayURL: `http://${containerHost}:${replayPort}${replayAdapter(provider).basePath}`,
  };
}

export function catalog(routes: Route[]): string {
  return stringify({
    providers: Object.fromEntries(routes.map(r => [r.instance, {
      type: r.provider, apiKeyEnv: providerKeyEnv, baseURL: r.replayURL,
      ...(r.provider === "openai-compatible" ? { providerName: "openai-compatible" } : {}),
    }])),
    models: Object.fromEntries(routes.map(r => [r.modelID, { name: r.rowID, primary: { provider: r.instance, model: r.model } }])),
  });
}

export function grafanaSettings(source: string): string {
  const header = `[${pluginSection}]`;
  const lines = source.split("\n");
  const start = lines.findIndex(line => line.trim() === header);
  if (start === -1) throw new Error(`grafana settings have no ${header} section`);
  const end = lines.findIndex((line, index) => index > start && line.trim().startsWith("["));
  const plugin = [header, `config_file = ${catalogMount}/models.yaml`, "access_mode = self-hosted", `${providerKeySetting} = ${backendKey}`];
  return [...lines.slice(0, start), ...plugin, "", ...(end === -1 ? [] : lines.slice(end))].join("\n");
}

export interface ProviderRejection {
  provider: string;
  evidence: string;
}

export function providerRejection(logs: string, routes: Route[]): ProviderRejection | undefined {
  for (const line of logs.split("\n")) {
    if (!line.includes("gateway plugin failed to start")) continue;
    const match = /providers\.([\w-]+)\.type \\?"([\w-]+)\\?" is unsupported/.exec(line);
    const rejected = match && routes.find(r => r.instance === match[1] && r.provider === match[2]);
    if (rejected) return { provider: rejected.provider, evidence: line };
  }
  return undefined;
}

interface ContainerState { Running: boolean; Status: string; ExitCode: number; OOMKilled: boolean; Error: string }

export class StackError extends Error {
  constructor(message: string, readonly evidence: Record<string, unknown>) { super(message); }
}

export interface StackOptions {
  image: string;
  pluginDirectory: string;
  settingsSource: string;
  provisioningSource: string;
  routes: Route[];
  signal: AbortSignal;
  run?: Run;
  fetch?: typeof fetch;
}

export interface Stack {
  url: string;
  token: string;
  adminPassword: string;
  admitted: Route[];
  rejected: Map<string, ProviderRejection>;
  boots: Array<{ providers: string[]; outcome: string }>;
  logs(since: Date, until?: Date): Promise<string>;
  alive(): Promise<boolean>;
  close(): Promise<void>;
}

export async function startStack(options: StackOptions): Promise<Stack> {
  const run = options.run ?? command;
  const http = options.fetch ?? fetch;
  const directory = mkdtempSync(join(tmpdir(), "gateway-conformance-stack-"));
  const name = `conformance-grafana-${randomUUID()}`;
  const adminPassword = randomUUID();
  const rejected = new Map<string, ProviderRejection>();
  const boots: Stack["boots"] = [];
  let admitted = options.routes;
  let running = false;
  const remove = async () => {
    if (!running) return;
    await removeContainer(name, run);
    running = false;
  };
  const logs = async (since?: Date, until?: Date) => {
    const window = [...(since ? ["--since", since.toISOString()] : []), ...(until ? ["--until", until.toISOString()] : [])];
    return run("docker", ["logs", ...window, name], { includeStderr: true });
  };
  const close = async () => {
    try { await remove(); } finally { rmSync(directory, { recursive: true, force: true }); }
  };
  try {
    stage(directory, options);
    for (;;) {
      const providers = [...new Set(admitted.map(r => r.provider))];
      if (!admitted.length) {
        return { url: "", token: "", adminPassword, admitted, rejected, boots, logs: async () => "", alive: async () => false, close };
      }
      writeFileSync(join(directory, "aigateway/models.yaml"), catalog(admitted), { mode: 0o644 });
      running = true;
      await run("docker", ["run", "--detach", "--name", name, "--add-host", `${containerHost}:host-gateway`, "--publish", `127.0.0.1::${containerPort.split("/")[0]}`,
        "--env", `GF_PATHS_CONFIG=${settingsMount}`, "--env", `GF_SECURITY_ADMIN_PASSWORD=${adminPassword}`,
        "--env", `GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=${pluginID}`, "--env", "GF_PLUGINS_PREINSTALL_DISABLED=true",
        "--env", "GF_ANALYTICS_REPORTING_ENABLED=false", "--env", "GF_ANALYTICS_CHECK_FOR_UPDATES=false", "--env", "GF_ANALYTICS_CHECK_FOR_PLUGIN_UPDATES=false",
        "--volume", `${join(directory, "conformance.ini")}:${settingsMount}:ro,z`,
        "--volume", `${join(directory, "provisioning")}:${provisioningMount}:ro,z`,
        "--volume", `${join(directory, "aigateway")}:${catalogMount}:ro,z`,
        "--volume", `${join(directory, "plugin")}:${pluginMount}:ro,z`,
        options.image], { signal: options.signal });
      const url = await waitForGrafana(name, run, http, options.signal);
      if (url) {
        boots.push({ providers, outcome: "ready" });
        const token = await serviceAccountToken(url, adminPassword, http);
        await waitForCatalog(url, token, admitted, http, options.signal);
        const alive = async () => {
          try {
            const state = await inspect(name, run);
            return state.Running && (await http(`${url}/api/health`, { signal: AbortSignal.timeout(2_000) })).ok;
          } catch { return false; }
        };
        return { url, token, adminPassword, admitted, rejected, boots, logs: async (since, until) => bounded(await logs(since, until)), alive, close };
      }
      const output = await logs();
      const state = await inspect(name, run);
      await remove();
      const rejection = !state.OOMKilled && !state.Error ? providerRejection(output, admitted) : undefined;
      if (!rejection) {
        boots.push({ providers, outcome: "failed" });
        throw new StackError(`grafana exited before readiness (exit ${state.ExitCode}); startup cause unclassified`, { state, logs: bounded(output), catalog: catalog(admitted) });
      }
      boots.push({ providers, outcome: `rejected ${rejection.provider}` });
      rejected.set(rejection.provider, rejection);
      admitted = admitted.filter(r => r.provider !== rejection.provider);
    }
  } catch (error) {
    if (!(error instanceof StackError) && running) {
      let output: string;
      try { output = bounded(await logs()); } catch (logError) { output = `startup logs unavailable: ${logError}`; }
      try { await close(); }
      catch (cleanupError) { throw new StackError(`gateway stack startup and cleanup failed: ${error}; ${cleanupError}`, { logs: output }); }
      throw new StackError(`gateway stack startup failed: ${error}`, { logs: output });
    }
    await close();
    throw error;
  }
}

function bounded(logs: string) {
  return logs.length > maxLogBytes ? logs.slice(-maxLogBytes) : logs;
}

function stage(directory: string, options: StackOptions) {
  chmodSync(directory, 0o755);
  for (const path of ["aigateway", "provisioning/plugins"]) mkdirSync(join(directory, path), { recursive: true, mode: 0o755 });
  writeFileSync(join(directory, "conformance.ini"), grafanaSettings(options.settingsSource), { mode: 0o644 });
  writeFileSync(join(directory, "provisioning/plugins/apps.yaml"), options.provisioningSource, { mode: 0o644 });
  cpSync(options.pluginDirectory, join(directory, "plugin"), { recursive: true });
}

async function inspect(name: string, run: Run): Promise<ContainerState> {
  return JSON.parse(await run("docker", ["inspect", "--format", "{{json .State}}", name])) as ContainerState;
}

async function removeContainer(name: string, run: Run) {
  try {
    await run("docker", ["rm", "--force", name]);
  } catch (error) {
    if (!String(error).includes(`No such container: ${name}`)) throw error;
  }
}

async function waitForGrafana(name: string, run: Run, http: typeof fetch, signal: AbortSignal): Promise<string | undefined> {
  const deadline = Date.now() + startupTimeout;
  let url = "";
  while (Date.now() < deadline) {
    signal.throwIfAborted();
    if (!(await inspect(name, run)).Running) return undefined;
    url ||= `http://${(await run("docker", ["port", name, containerPort])).trim().split("\n")[0]}`;
    try {
      if ((await http(`${url}/api/health`, { signal: AbortSignal.timeout(2_000) })).ok) return url;
    } catch (error) {
      if (signal.aborted) throw error;
    }
    await delay(pollInterval, undefined, { signal });
  }
  throw new Error("grafana readiness timed out");
}

async function serviceAccountToken(url: string, adminPassword: string, http: typeof fetch): Promise<string> {
  const authorization = `Basic ${Buffer.from(`admin:${adminPassword}`).toString("base64")}`;
  const post = async (path: string, body: unknown) => {
    const response = await http(`${url}${path}`, { method: "POST", headers: { authorization, "content-type": "application/json" }, body: JSON.stringify(body), signal: AbortSignal.timeout(10_000) });
    const text = await response.text();
    if (!response.ok) throw new Error(`${path} returned ${response.status}: ${text.slice(0, 500)}`);
    return JSON.parse(text);
  };
  const account = await post("/api/serviceaccounts", { name: "conformance", role: "Viewer" }) as { id: number };
  return (await post(`/api/serviceaccounts/${account.id}/tokens`, { name: "conformance" }) as { key: string }).key;
}

async function waitForCatalog(url: string, token: string, routes: Route[], http: typeof fetch, signal: AbortSignal) {
  const deadline = Date.now() + catalogTimeout;
  let last = "";
  while (Date.now() < deadline) {
    signal.throwIfAborted();
    try {
      const response = await http(`${url}${routePrefix}/config`, { headers: { authorization: `Bearer ${token}` }, signal: AbortSignal.timeout(5_000) });
      const body = await response.text();
      last = `${response.status} ${body.slice(0, 500)}`;
      if (response.ok) {
        const listed = new Set((JSON.parse(body) as { models: Array<{ id: string }> }).models.map(model => model.id));
        const missing = routes.filter(r => !listed.has(r.modelID)).map(r => r.modelID);
        if (!missing.length) return;
        last = `catalog is missing ${missing.length} model(s): ${missing.slice(0, 5).join(", ")}`;
      }
    } catch (error) {
      if (signal.aborted) throw error;
      last = String(error);
    }
    await delay(pollInterval, undefined, { signal });
  }
  throw new Error(`gateway catalog did not become ready: ${last}`);
}
