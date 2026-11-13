import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { parse } from "yaml";
import { catalog, command, grafanaSettings, providerRejection, route, routePrefix, StackError, startStack, type Route, type Run } from "./gateway-runtime.mts";

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

test("catalog shares one instance per provider and one model per backend model", () => {
  assert.throws(() => route("future/x/go", "future", { model: "m" }, 1), /missing replay\/configuration adapter/);
  const providers = ["anthropic", "openai", "openai-compatible", "bedrock"];
  const routes = providers.flatMap((provider, index) => ["go", "typescript"].map(client => route(`${provider}/recorded/text/${client}`, provider, { model: `${provider}-model` }, 4000 + index)));
  const config = parse(catalog(routes));
  assert.deepEqual(Object.keys(config.providers), providers);
  assert.equal(Object.keys(config.models).length, providers.length);
  for (const [index, provider] of providers.entries()) {
    const backend = config.providers[provider];
    assert.equal(backend.type, provider);
    assert.equal(backend.apiKeyEnv, "GF_PLUGIN_CONFORMANCE_PROVIDER_KEY");
    assert.equal(backend.baseURL, `http://host.docker.internal:${4000 + index}${["openai", "openai-compatible"].includes(provider) ? "/v1" : ""}`);
    assert.equal(backend.providerName, provider === "openai-compatible" ? "openai-compatible" : undefined);
    assert.equal(backend.supportsStructuredOutputs, provider === "openai-compatible" ? true : undefined);
    const modelID = `conformance/${provider}/${provider}-model`;
    assert.deepEqual(config.models[modelID], { name: modelID, primary: { provider, model: `${provider}-model` } });
  }
  assert.throws(() => catalog([route("a/go", "anthropic", { model: "m" }, 1), route("b/go", "anthropic", { model: "m" }, 2)]), /more than one replay URL/);
});

test("catalog admits the union of server fallbacks and skills its fixtures request", () => {
  const anthropic = (providerOptions: Record<string, unknown>, model = "claude") => route("a/go", "anthropic", { model, providerOptions: { anthropic: providerOptions } }, 1);
  const config = parse(catalog([
    anthropic({ fallbacks: [{ model: "b", max_tokens: 1 }, { model: "a" }] }),
    anthropic({ fallbacks: "default" }),
    anthropic({ container: { skills: [{ type: "anthropic", skillId: "pptx" }, { type: "custom", skillId: "s", version: "2" }] } }),
    anthropic({ container: { skills: [{ type: "custom", skillId: "s", version: "1" }] } }),
    anthropic({}),
    anthropic({ fallbacks: "default" }, "other"),
    route("o/go", "openai", { model: "claude", providerOptions: { anthropic: { fallbacks: "default" } } }, 2),
  ]));
  assert.deepEqual(config.models["conformance/anthropic/claude"].primary, {
    provider: "anthropic", model: "claude",
    serverFallbacks: { models: ["a", "b"], allowDefault: true },
    skills: { allow: [{ type: "anthropic", skillId: "pptx" }, { type: "custom", skillId: "s", versions: ["1", "2"] }] },
  });
  assert.deepEqual(config.models["conformance/anthropic/other"].primary, { provider: "anthropic", model: "other", serverFallbacks: { allowDefault: true } });
  assert.deepEqual(config.models["conformance/openai/claude"].primary, { provider: "openai", model: "claude" });
});

test("settings keep the source toggles and replace only the plugin section", () => {
  const source = "; comment\n[feature_toggles]\nappplugins.registerAPIServer = true\n\n[plugin.grafana-aigateway-app]\nmodel_catalog = /dev\nanthropic_api_key = $__env{KEY}\n\n[log]\nlevel = info\n";
  const settings = grafanaSettings(source);
  assert.match(settings, /\[feature_toggles\]\nappplugins.registerAPIServer = true/);
  assert.match(settings, /model_catalog = \$__file\{\/etc\/grafana\/aigateway\/models.yaml\}\naccess_mode = self-hosted\nconformance_provider_key = conformance-backend-key/);
  assert.doesNotMatch(settings, /anthropic_api_key|\/dev/);
  assert.match(settings, /\[log\]\nlevel = info/);
  assert.throws(() => grafanaSettings("[feature_toggles]\n"), /no \[plugin.grafana-aigateway-app\] section/);
});

test("provider rejection needs the plugin startup reason for a configured instance", () => {
  const routes = [route("bedrock/recorded/a/go", "bedrock", { model: "m" }, 1)];
  const line = 'logger=plugin.grafana-aigateway-app level=error msg="gateway plugin failed to start" class=startup_failure reason="config: providers.bedrock.type \\"bedrock\\" is unsupported (want anthropic, openai or openai-compatible)"';
  assert.deepEqual(providerRejection(`before\n${line}\nafter`, routes), { provider: "bedrock", evidence: line });
  assert.equal(providerRejection(line.replace("providers.bedrock", "providers.other"), routes), undefined);
  assert.equal(providerRejection(line.replace("gateway plugin failed to start", "other"), routes), undefined);
  assert.equal(providerRejection("plugin process exited", routes), undefined);
});

function fakeStack(boots: Array<{ running: boolean; logs?: string }>) {
  const calls: string[][] = [];
  let boot = -1;
  const run: Run = async (_file, args) => {
    calls.push(args);
    switch (args[0]) {
      case "run": boot++; return "";
      case "inspect": return JSON.stringify({ Running: boots[boot].running, Status: "", ExitCode: boots[boot].running ? 0 : 1, OOMKilled: false, Error: "" });
      case "port": return "127.0.0.1:43210\n";
      case "logs": return boots[boot].logs ?? "";
      default: return "";
    }
  };
  let catalogIDs: string[] = [];
  const http = (async (input: string | URL | Request) => {
    const path = new URL(String(input)).pathname;
    if (path === "/api/serviceaccounts") return Response.json({ id: 7 });
    if (path === "/api/serviceaccounts/7/tokens") return Response.json({ key: "service-token" });
    if (path === `${routePrefix}/config`) return Response.json({ models: catalogIDs.map(id => ({ id })) });
    return new Response("ok");
  }) as typeof fetch;
  return { calls, run, http, setCatalog: (routes: Route[]) => { catalogIDs = routes.map(r => r.modelID); } };
}

function withPlugin<T>(body: (directory: string) => Promise<T>) {
  const directory = mkdtempSync(join(tmpdir(), "gateway-plugin-"));
  writeFileSync(join(directory, "plugin.json"), "{}");
  return body(directory).finally(() => rmSync(directory, { recursive: true, force: true }));
}

const settingsSource = "[feature_toggles]\n[plugin.grafana-aigateway-app]\n";
const routes = [route("anthropic/recorded/a/go", "anthropic", { model: "m" }, 1), route("bedrock/recorded/b/go", "bedrock", { model: "m" }, 2), route("bedrock/recorded/c/go", "bedrock", { model: "n" }, 2)];
const rejectionLog = 'msg="gateway plugin failed to start" reason="config: providers.bedrock.type \\"bedrock\\" is unsupported"';

test("stack drops an explicitly rejected provider and boots once more", () => withPlugin(async pluginDirectory => {
  const fake = fakeStack([{ running: false, logs: rejectionLog }, { running: true }]);
  fake.setCatalog(routes.slice(0, 1));
  const stack = await startStack({ image: "grafana", pluginDirectory, settingsSource, provisioningSource: "apps", routes, signal: new AbortController().signal, run: fake.run, fetch: fake.http });
  try {
    assert.equal(stack.url, "http://127.0.0.1:43210");
    assert.equal(stack.token, "service-token");
    assert.deepEqual(stack.admitted.map(r => r.instance), ["anthropic"]);
    assert.deepEqual([...stack.rejected.keys()], ["bedrock"]);
    assert.deepEqual(stack.boots.map(b => b.outcome), ["rejected bedrock", "ready"]);
    assert.equal(fake.calls.filter(args => args[0] === "run").length, 2);
    const firstRemove = fake.calls.findIndex(args => args[0] === "rm");
    assert.ok(firstRemove > fake.calls.findIndex(args => args[0] === "logs"), "startup logs are read before removal");
    const run = fake.calls.find(args => args[0] === "run")!;
    for (const flag of ["GF_PLUGINS_PREINSTALL_DISABLED=true", "GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=grafana-aigateway-app", "host.docker.internal:host-gateway", "127.0.0.1::3000"]) assert.ok(run.includes(flag), flag);
  } finally { await stack.close(); }
  assert.equal(fake.calls.filter(args => args[0] === "rm").length, 2);
}));

test("rejection evidence survives verbose shutdown logs", () => withPlugin(async pluginDirectory => {
  const fake = fakeStack([{ running: false, logs: `${rejectionLog}\n${"shutdown noise\n".repeat(20_000)}` }, { running: true }]);
  fake.setCatalog(routes.slice(0, 1));
  const stack = await startStack({ image: "grafana", pluginDirectory, settingsSource, provisioningSource: "apps", routes, signal: new AbortController().signal, run: fake.run, fetch: fake.http });
  try { assert.deepEqual([...stack.rejected.keys()], ["bedrock"]); } finally { await stack.close(); }
}));

test("unclassified startup exits fail the whole stack with evidence", () => withPlugin(async pluginDirectory => {
  const fake = fakeStack([{ running: false, logs: "panic: something else" }]);
  await assert.rejects(
    startStack({ image: "grafana", pluginDirectory, settingsSource, provisioningSource: "apps", routes, signal: new AbortController().signal, run: fake.run, fetch: fake.http }),
    (error: unknown) => error instanceof StackError && /startup cause unclassified/.test(error.message) && error.evidence.logs === "panic: something else",
  );
  assert.equal(fake.calls.filter(args => args[0] === "rm").length, 1);
}));

test("stack with every provider rejected does not boot again", () => withPlugin(async pluginDirectory => {
  const fake = fakeStack([{ running: false, logs: rejectionLog }]);
  const stack = await startStack({ image: "grafana", pluginDirectory, settingsSource, provisioningSource: "apps", routes: routes.slice(1), signal: new AbortController().signal, run: fake.run, fetch: fake.http });
  assert.deepEqual(stack.admitted, []);
  assert.equal(await stack.alive(), false);
  assert.equal(fake.calls.filter(args => args[0] === "run").length, 1);
  await stack.close();
}));

test("an ambiguous container creation failure still removes the named container", () => withPlugin(async pluginDirectory => {
  const fake = fakeStack([{ running: true }]);
  const run: Run = async (file, args, options) => {
    if (args[0] === "run") { await fake.run(file, args, options); throw new Error("command timed out: docker"); }
    return fake.run(file, args, options);
  };
  await assert.rejects(startStack({ image: "grafana", pluginDirectory, settingsSource, provisioningSource: "apps", routes, signal: new AbortController().signal, run, fetch: fake.http }), /timed out/);
  const name = fake.calls.find(args => args[0] === "run")![3];
  assert.ok(fake.calls.some(args => args[0] === "rm" && args.includes(name)));
}));

test("cancellation during readiness removes the container and keeps logs", () => withPlugin(async pluginDirectory => {
  const fake = fakeStack([{ running: true, logs: "booting" }]);
  const controller = new AbortController();
  const http = (async () => { controller.abort(); throw new Error("connection refused"); }) as typeof fetch;
  await assert.rejects(
    startStack({ image: "grafana", pluginDirectory, settingsSource, provisioningSource: "apps", routes, signal: controller.signal, run: fake.run, fetch: http }),
    (error: unknown) => error instanceof StackError && error.evidence.logs === "booting",
  );
  assert.ok(fake.calls.some(args => args[0] === "rm"));
}));
