import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import nodeProcess from "node:process";

export function buildGoClientCapture(directory: string, sourceDirectory = resolve(import.meta.dirname, "../../../providers/grafana")): string {
  const root = resolve(import.meta.dirname, "../../..");
  const rootClient = join(root, "providers/grafana");
  let workspace = join(root, "go.work");
  if (sourceDirectory !== rootClient) {
    const sourceWorkspace = JSON.parse(execFileSync("go", ["work", "edit", "-json", workspace], { encoding: "utf8" })) as { Go: string; Use: Array<{ DiskPath: string }> };
    workspace = join(directory, "go.work");
    const modules = sourceWorkspace.Use.map(({ DiskPath }) => resolve(root, DiskPath)).filter((path) => path !== rootClient);
    writeFileSync(workspace, `go ${sourceWorkspace.Go}\n\nuse (\n${[...modules, sourceDirectory].map((path) => `\t${path}`).join("\n")}\n)\n`);
  }
  const env = { ...nodeProcess.env, GOWORK: workspace, GOFLAGS: `${nodeProcess.env.GOFLAGS ? `${nodeProcess.env.GOFLAGS} ` : ""}-mod=readonly` };
  for (const [module, path] of [
    ["github.com/grafana/ai-sdk", root],
    ["github.com/grafana/ai-sdk/providers/grafana", sourceDirectory],
  ] as const) {
    const selected = JSON.parse(execFileSync("go", ["list", "-m", "-json", module], { cwd: sourceDirectory, env, encoding: "utf8" }));
    assert.equal(selected.Dir, path, "Go client capture must select candidate source");
    assert.equal(selected.Main, true);
  }
  const binary = join(directory, "go-client-capture");
  execFileSync("go", ["build", "-o", binary, "./internal/capture"], { cwd: sourceDirectory, env, stdio: "pipe" });
  return binary;
}

export function buildPinnedGoClientCapture(directory: string): string {
  const binary = join(directory, "go-client-capture");
  execFileSync("go", ["build", "-o", binary, "./internal/capture"], {
    cwd: resolve(import.meta.dirname, "../../../providers/grafana"),
    env: { ...nodeProcess.env, GOWORK: "off", GOFLAGS: "-mod=readonly" },
    stdio: "pipe",
  });
  return binary;
}

export function buildGoStreamTextCapture(directory: string): string {
  const root = resolve(import.meta.dirname, "../../..");
  const env = { ...nodeProcess.env, GOWORK: join(root, "go.work"), GOFLAGS: "-mod=readonly" };
  for (const [module, path] of [
    ["github.com/grafana/ai-sdk", root],
    ["github.com/grafana/ai-sdk/providers/grafana", join(root, "providers/grafana")],
  ] as const) {
    const selected = JSON.parse(execFileSync("go", ["list", "-m", "-json", module], { cwd: root, env, encoding: "utf8" }));
    assert.equal(selected.Dir, path, "high-level probe must use the changed local source");
    assert.equal(selected.Main, true);
  }
  const binary = join(directory, "go-stream-text-capture");
  execFileSync("go", ["build", "-o", binary, "./providers/grafana/internal/capture"], {
    cwd: root, env, stdio: "pipe",
  });
  return binary;
}

export function captureGoClient(binary: string, input: Record<string, unknown>): Promise<Record<string, any>> {
  return new Promise((resolve, reject) => {
    const child = spawn(binary, [], { stdio: ["pipe", "pipe", "pipe"] });
    let stdout = "";
    let stderr = "";
    const timeout = setTimeout(() => child.kill("SIGKILL"), 15_000);
    child.stdout.on("data", (data: Buffer) => { stdout += data.toString(); });
    child.stderr.on("data", (data: Buffer) => { stderr += data.toString(); });
    child.once("error", (error) => { clearTimeout(timeout); reject(error); });
    child.once("close", (code) => {
      clearTimeout(timeout);
      if (code !== 0) { reject(new Error(`Go capture failed: ${code}: ${stderr}`)); return; }
      try { resolve(JSON.parse(stdout)); } catch (error) { reject(error); }
    });
    child.stdin.end(JSON.stringify(input));
  });
}
