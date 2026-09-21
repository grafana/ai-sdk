import assert from "node:assert/strict";
import { createPublicKey, verify } from "node:crypto";
import { test } from "node:test";
import { parse } from "yaml";
import { authentication, command, createNetwork, gatewayConfig, removeResource, startAuthentication, startGateway } from "./gateway-runtime.mts";

test("commands fail on exit, timeout, cancellation and excessive output", async () => {
  assert.equal(await command(process.execPath, ["-e", "process.stdout.write('ok')"]), "ok");
  await assert.rejects(command(process.execPath, ["-e", "process.exit(7)"]), /exited 7/);
  await assert.rejects(command(process.execPath, ["-e", "setInterval(() => {}, 1000)"], { timeout: 30 }), /timed out/);
  const controller = new AbortController();
  controller.abort();
  await assert.rejects(command(process.execPath, ["-e", "setInterval(() => {}, 1000)"], { signal: controller.signal }), /canceled/);
  await assert.rejects(command(process.execPath, ["-e", "process.stdout.write('x'.repeat(5*1024*1024))"]), /exceeded limit/);
  assert.equal(await command(process.execPath, ["-e", "process.stderr.write('evidence')"], { includeStderr: true }), "evidence");
});

test("authentication serves an ephemeral verifiable access token without unsafe mode", async () => {
  const auth = authentication();
  const server = await startAuthentication("127.0.0.1", auth.jwks);
  try {
    const jwks = await (await fetch(server.url)).json();
    const [header, payload, signature] = auth.token.split(".");
    assert.equal(JSON.parse(Buffer.from(header, "base64url").toString()).typ, "at+jwt");
    assert.equal(verify("sha256", Buffer.from(`${header}.${payload}`), { key: createPublicKey({ key: jwks.keys[0], format: "jwk" }), dsaEncoding: "ieee-p1363" }, Buffer.from(signature, "base64url")), true);
  } finally { await server.close(); }
  await assert.rejects(fetch(server.url));
});

test("configuration never substitutes unsupported provider protocols", () => {
  assert.throws(() => gatewayConfig({ provider: "future", dir: "unused", name: "new" }, { model: "test" }, "http://replay"), /missing replay\/configuration adapter/);
  for (const provider of ["anthropic", "openai-compatible", "openai", "bedrock"]) {
    const config = parse(gatewayConfig({ provider, dir: "unused", name: provider }, { model: "native-model" }, "http://replay:1234"));
    assert.equal(config.providers.replay.type, provider);
    assert.equal(config.models["conformance/model"].primary.model, "native-model");
    assert.equal(config.providers.replay.apiKeyEnv, "CONFORMANCE_PROVIDER_KEY");
    assert.equal(config.providers.replay.baseURL, `http://replay:1234${["openai", "openai-compatible"].includes(provider) ? "/v1" : ""}`);
  }
});

test("ambiguous Docker creation failures still remove owned resources", async () => {
  const calls: string[][] = [];
  const run: typeof command = async (_file, args) => {
    calls.push(args);
    if (args.includes("create")) throw new Error("create response timed out");
    return "";
  };
  await assert.rejects(createNetwork(run), /timed out/);
  assert.ok(calls.some(args => args[0] === "network" && args[1] === "rm"));
  calls.length = 0;
  await assert.rejects(startGateway("image", "network", "config", "http://jwks", new AbortController().signal, run), /timed out/);
  assert.ok(calls.some(args => args[0] === "rm"));
});

test("startup cancellation retains logs before cleanup", async () => {
  const calls: string[][] = [];
  const run: typeof command = async (_file, args) => { calls.push(args); return args[0] === "logs" ? "startup evidence" : ""; };
  const controller = new AbortController();
  controller.abort(new Error("canceled readiness"));
  await assert.rejects(startGateway("image", "network", "config", "http://jwks", controller.signal, run), /startup evidence/);
  assert.ok(calls.findIndex(args => args[0] === "logs") < calls.findIndex(args => args[0] === "rm"));
});

test("startup rejection needs evidence and crashes stay harness failures", async () => {
  for (const tc of [
    { logs: "gateway process failed", oom: false, stage: "harness" },
    { logs: 'config: providers.replay.type "bedrock" is unsupported', oom: true, stage: "harness" },
    { logs: 'config: providers.replay.type "bedrock" is unsupported', oom: false, stage: "provider-setup" },
  ]) {
    const run: typeof command = async (_file, args) => {
      if (args[0] === "inspect") return JSON.stringify({ Running: false, ExitCode: 1, OOMKilled: tc.oom, Error: "" });
      return args[0] === "logs" ? tc.logs : "";
    };
    const result = await startGateway("image", "network", "config", "http://jwks", new AbortController().signal, run);
    try {
      assert.equal(result.failureStage, tc.stage);
      assert.equal(result.startupLogs, tc.logs);
    } finally { await result.close(); }
  }
});

test("cleanup tolerates only an absent owned resource", async () => {
  await removeResource("container", "owned", async () => { throw new Error("No such container: owned"); });
  await removeResource("network", "owned", async () => { throw new Error("network owned not found"); });
  await assert.rejects(removeResource("container", "owned", async () => { throw new Error("daemon unavailable"); }), /daemon unavailable/);
});
