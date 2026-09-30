import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { once } from "node:events";
import { cpSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { basename, join, resolve } from "node:path";
import { it } from "node:test";

const root = resolve(import.meta.dirname, "../../..");
const marker = "workspace-candidate-user-agent";
const docker = (args: string[], cwd = root) => {
  const result = spawnSync("docker", args, { cwd, encoding: "utf8", timeout: 300_000 });
  assert.equal(result.status, 0, `${args.join(" ")}\n${result.stdout}\n${result.stderr}`);
  return result.stdout;
};

it("builds a local SDK/provider change into the Gateway image with honest source and license evidence", { timeout: 600_000 }, async (t) => {
  if (process.platform !== "linux" || spawnSync("docker", ["info"], { stdio: "ignore" }).status !== 0) {
    if (process.env.CI) assert.fail("Docker is required for the Gateway image regression in CI");
    t.skip("Docker on Linux is required");
    return;
  }

  const temp = mkdtempSync(join(tmpdir(), "gateway-image-source-"));
  const image = `ai-gateway:source-regression-${process.pid}`;
  const buildImage = `${image}-build`;
  const containerName = `gateway-source-regression-${process.pid}`;
  let observedPath = "";
  let observedAgent = "";
  const backend = createServer((request, response) => {
    observedPath = request.url ?? "";
    observedAgent = request.headers["user-agent"] ?? "";
    response.writeHead(200, { "content-type": "application/json" });
    response.end(JSON.stringify({
      id: "chatcmpl-test",
      object: "chat.completion",
      created: 1,
      model: "test",
      choices: [{ index: 0, message: { role: "assistant", content: "candidate" }, finish_reason: "stop" }],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
    }));
  });
  let processExited = false;
  let logs = "";
  try {
    for (const file of ["go.gateway.work", "go.gateway.work.sum", "go.mod", "go.sum", "LICENSE", "NOTICE"]) {
      cpSync(join(root, file), join(temp, file));
    }
    for (const file of readdirSync(root).filter((name) => name.endsWith(".go") && !name.endsWith("_test.go"))) {
      cpSync(join(root, file), join(temp, file));
    }
    const sourceDirs = ["ai-gateway", "provider", "fallback", "output", "registry", "schema", "internal", "middleware", "providers"];
    const assets = ["go.mod", "go.sum", "LICENSE", "NOTICE", "Dockerfile", "Dockerfile.dockerignore", "request.json", "provider_tool_schemas.json"];
    for (const dir of sourceDirs) {
      cpSync(join(root, dir), join(temp, dir), {
        recursive: true,
        filter: (path) => {
          if (path === join(root, dir)) return true;
          const name = basename(path);
          if (["node_modules", ".git", "test", "docs"].includes(name)) return false;
          if (!name.includes(".")) return true;
          return (name.endsWith(".go") && !name.endsWith("_test.go")) || assets.includes(name);
        },
      });
    }
    const providerFile = join(temp, "providers/openai-compatible/model.go");
    const before = readFileSync(providerFile, "utf8");
    assert.match(before, /userAgent\s*= "ai-sdk-go\/openai-compatible"/);
    writeFileSync(providerFile, before.replace('userAgent            = "ai-sdk-go/openai-compatible"', `userAgent            = "${marker}"`));
    writeFileSync(join(temp, ".env"), "SENTINEL_SECRET=not-for-docker\n");
    mkdirSync(join(temp, "node_modules"));
    writeFileSync(join(temp, "node_modules/sentinel"), "not-for-docker\n");
    const manifests = ["go.gateway.work", "go.gateway.work.sum", "go.mod", "go.sum", "ai-gateway/go.mod", "ai-gateway/go.sum"];
    const originalManifests = manifests.map((path) => readFileSync(join(temp, path), "utf8"));
    backend.listen(0, "127.0.0.1");
    await once(backend, "listening");
    const backendPort = (backend.address() as { port: number }).port;
    docker(["build", "-f", "ai-gateway/Dockerfile", "--build-arg", "VCS_REF=local-unverified", "-t", image, "."], temp);
    for (const [index, path] of manifests.entries()) {
      assert.equal(readFileSync(join(temp, path), "utf8"), originalManifests[index]);
    }
    assert.equal(docker(["image", "inspect", "--format", "{{index .Config.Labels \"org.opencontainers.image.revision\"}}", image]).trim(), "local-unverified");
    docker(["build", "--target", "build", "-f", "ai-gateway/Dockerfile", "--build-arg", "VCS_REF=local-unverified", "-t", buildImage, "."], temp);
    docker(["run", "--rm", "--entrypoint", "/bin/sh", buildImage, "-c", "test ! -e /src/.env && test ! -e /src/node_modules && test -f /src/ai-gateway/providerwire/v4/schema/request.json"]);
    for (const [index, path] of manifests.entries()) {
      assert.equal(docker(["run", "--rm", "--entrypoint", "/bin/cat", buildImage, `/src/${path}`]), originalManifests[index]);
    }
    const inventory = docker(["run", "--rm", "--entrypoint", "/bin/cat", image, "/usr/share/licenses/grafana-ai-gateway/THIRD_PARTY_MODULES.txt"]);
    assert.match(inventory, /github\.com\/grafana\/ai-sdk\/providers\/openai-compatible\tlocal-unverified\t-/);
    assert.match(inventory, /github\.com\/openai\/openai-go\/v3\tv[\d.]+\th1:/);
    for (const location of ["dependencies/github.com/grafana/ai-sdk@local-unverified/LICENSE", "dependencies/github.com/grafana/ai-sdk/ai-gateway@local-unverified/LICENSE", "grafana-ai-gateway/NOTICE"]) {
      docker(["run", "--rm", "--entrypoint", "/bin/test", image, "-f", `/usr/share/licenses/grafana-ai-gateway/${location}`]);
    }
    const config = join(temp, "models.yaml");
    writeFileSync(config, [
      "providers:",
      "  local:",
      "    type: openai-compatible",
      "    apiKeyEnv: TEST_API_KEY",
      `    baseURL: http://127.0.0.1:${backendPort}/v1`,
      "models:",
      "  public/model:",
      "    name: Fixture",
      "    primary:",
      "      provider: local",
      "      model: test",
      "",
    ].join("\n"));
    const portReservation = createServer();
    portReservation.listen(0, "127.0.0.1");
    await once(portReservation, "listening");
    const gatewayPort = (portReservation.address() as { port: number }).port;
    portReservation.close();
    const run = spawn("docker", [
      "run", "--rm", "--name", containerName, "--network", "host",
      "--mount", `type=bind,source=${config},target=/etc/gateway/models.yaml,readonly`,
      "-e", "TEST_API_KEY=fixture", image,
      "--deployment.mode=development", "--auth.mode=cloud-gateway",
      "--config.file=/etc/gateway/models.yaml",
      `--server.listen-address=127.0.0.1:${gatewayPort}`,
      "--server.operational-listen-address=127.0.0.1:0",
    ], { stdio: ["ignore", "pipe", "pipe"] });
    run.stdout.on("data", (chunk) => { logs += chunk; });
    run.stderr.on("data", (chunk) => { logs += chunk; });
    run.on("exit", () => { processExited = true; });
    let ready = false;
    for (let attempt = 0; attempt < 60; attempt++) {
      if (processExited) break;
      try {
        ready = (await fetch(`http://127.0.0.1:${gatewayPort}/api/v1/aisdk/config`, { headers: { "x-scope-orgid": "1" } })).ok;
      } catch {}
      if (ready) break;
      await new Promise((done) => setTimeout(done, 250));
    }
    assert.ok(ready, `gateway failed to start: ${logs}`);
    const result = await fetch(`http://127.0.0.1:${gatewayPort}/api/v1/aisdk/language-model`, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        "ai-language-model-specification-version": "4",
        "ai-language-model-id": "public/model",
        "ai-language-model-streaming": "false",
        "x-scope-orgid": "1",
      },
      body: JSON.stringify({ prompt: [{ role: "user", content: [{ type: "text", text: "hello" }] }] }),
    });
    assert.equal(result.status, 200, `${await result.text()}\n${logs}`);
    assert.equal(observedPath, "/v1/chat/completions");
    assert.equal(observedAgent, marker);
  } finally {
    backend.close();
    spawnSync("docker", ["rm", "-f", containerName], { stdio: "ignore" });
    spawnSync("docker", ["image", "rm", "-f", image, buildImage], { stdio: "ignore" });
    rmSync(temp, { recursive: true, force: true });
  }
});
