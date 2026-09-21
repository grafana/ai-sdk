import { spawn } from "node:child_process";
import { generateKeyPairSync, randomUUID, sign } from "node:crypto";
import { createServer } from "node:http";
import { mkdtempSync, chmodSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { stringify } from "yaml";
import type { Config, TestCase } from "./common.mts";
import { replayAdapter } from "./replay.mts";

const commandTimeout = 30_000;
const maxCommandOutput = 4 * 1024 * 1024;
export const backendKey = "conformance-backend-key";
export const gatewayAPIKey = "conformance-gateway-key";
export const modelID = "conformance/model";

export function authentication() {
  const { privateKey, publicKey } = generateKeyPairSync("ec", { namedCurve: "P-256" });
  const keyID = "conformance";
  const header = Buffer.from(JSON.stringify({ alg: "ES256", typ: "at+jwt", kid: keyID })).toString("base64url");
  const payload = Buffer.from(JSON.stringify({ sub: "access-policy:conformance", aud: ["ai-sdk"], exp: Math.floor(Date.now() / 1000) + 86400, namespace: "stack-conformance", serviceIdentity: "conformance" })).toString("base64url");
  const signed = `${header}.${payload}`;
  const signature = sign("sha256", Buffer.from(signed), { key: privateKey, dsaEncoding: "ieee-p1363" }).toString("base64url");
  return { token: `${signed}.${signature}`, jwks: { keys: [{ ...publicKey.export({ format: "jwk" }), kid: keyID, use: "sig", alg: "ES256" }] } };
}

export async function startAuthentication(host: string, jwks: unknown) {
  const server = createServer((_req, res) => res.writeHead(200, { "content-type": "application/json" }).end(JSON.stringify(jwks)));
  await new Promise<void>((resolve, reject) => { server.once("error", reject); server.listen(0, host, () => { server.removeListener("error", reject); resolve(); }); });
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("missing authentication listener");
  return {
    url: `http://${host}:${address.port}`,
    close: () => new Promise<void>((resolve, reject) => { server.close(error => error ? reject(error) : resolve()); server.closeAllConnections(); }),
  };
}

export function command(file: string, args: string[], options: { cwd?: string; input?: unknown; timeout?: number; signal?: AbortSignal; includeStderr?: boolean } = {}): Promise<string> {
  return new Promise((resolve, reject) => {
    const env = Object.fromEntries(["PATH", "HOME", "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_CONFIG", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH", "GOTOOLCHAIN", "GOCACHE", "GOMODCACHE"].flatMap(key => process.env[key] ? [[key, process.env[key]!]] : []));
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

export function gatewayConfig(tc: TestCase, cfg: Config, replayURL: string) {
  const baseURL = replayURL + replayAdapter(tc.provider).basePath;
  return stringify({
    providers: { replay: { type: tc.provider, apiKeyEnv: "CONFORMANCE_PROVIDER_KEY", baseURL, ...(tc.provider === "openai-compatible" ? { providerName: "openai-compatible" } : {}) } },
    models: { [modelID]: { name: "Conformance", primary: { provider: "replay", model: cfg.model } } },
  });
}

export async function removeResource(kind: "network" | "container", name: string, run = command) {
  try {
    await run("docker", kind === "network" ? ["network", "rm", name] : ["rm", "--force", name]);
  } catch (error) {
    const message = String(error);
    if (!(kind === "container" ? message.includes(`No such container: ${name}`) : message.includes(`network ${name} not found`))) throw error;
  }
}

export async function createNetwork(run = command) {
  if (process.platform !== "linux") throw new Error("gateway conformance currently requires Linux with a local Docker daemon");
  const name = `conformance-${randomUUID()}`;
  try {
    await run("docker", ["network", "create", "--internal", name]);
    const host = (await run("docker", ["network", "inspect", "--format", "{{(index .IPAM.Config 0).Gateway}}", name])).trim();
    return { name, host, close: () => removeResource("network", name, run) };
  } catch (error) {
    await removeResource("network", name, run);
    throw error;
  }
}

export async function startGateway(image: string, network: string, configuration: string, jwksURL: string, signal: AbortSignal, run = command) {
  const directory = mkdtempSync(join(tmpdir(), "gateway-conformance-"));
  chmodSync(directory, 0o755);
  const path = join(directory, "models.yaml");
  writeFileSync(path, configuration, { mode: 0o644 });
  const name = `conformance-${randomUUID()}`;
  const close = async () => {
    try {
      await removeResource("container", name, run);
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  };
  const logs = () => run("docker", ["logs", "--tail", "200", name], { includeStderr: true });
  try {
    await run("docker", ["create", "--name", name, "--network", network, "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--log-driver", "local", "--log-opt", "max-size=1m", "--log-opt", "max-file=2", "--mount", `type=bind,source=${path},target=/models.yaml,readonly`, "--env", `CONFORMANCE_PROVIDER_KEY=${backendKey}`, image, "--config.file=/models.yaml", "--deployment.mode=development", `--auth.jwks-url=${jwksURL}`, "--server.listen-address=0.0.0.0:8080", "--server.shutdown-timeout=2s"]);
    await run("docker", ["start", name]);
    let url = "";
    const deadline = Date.now() + 15_000;
    while (Date.now() < deadline) {
      signal.throwIfAborted();
      const state = JSON.parse(await run("docker", ["inspect", "--format", "{{json .State}}", name])) as { Running: boolean; ExitCode: number; OOMKilled: boolean; Error: string };
      if (!state.Running) {
        const output = await logs();
        const providerRejection = !state.OOMKilled && !state.Error && /providers\.[\w-]+\.type .+ is unsupported/.test(output);
        return { url, close, logs, failureStage: providerRejection ? "provider-setup" as const : "harness" as const, setupError: `gateway exited before readiness (exit ${state.ExitCode}); ${providerRejection ? "provider configuration rejected" : "startup cause unclassified"}`, startupState: state, startupLogs: output };
      }
      if (!url) {
        const address = (await run("docker", ["inspect", "--format", `{{(index .NetworkSettings.Networks "${network}").IPAddress}}`, name])).trim();
        url = `http://${address}:8080`;
      }
      try {
        if ((await fetch(`${url}/ready`, { signal: AbortSignal.timeout(500) })).ok) return { url, close, logs };
      } catch (error) {
        if (signal.aborted) throw error;
      }
      await delay(50, undefined, { signal });
    }
    throw new Error("gateway readiness timed out");
  } catch (error) {
    let output: string;
    try { output = await logs(); }
    catch (logError) { output = `startup logs unavailable: ${logError}`; }
    try { await close(); }
    catch (cleanupError) { throw new AggregateError([error, cleanupError], `gateway startup and cleanup failed\n${output}`); }
    throw new Error(`${error}\n${output}`);
  }
}
