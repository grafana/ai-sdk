import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { realpathSync } from "node:fs";
import { join, resolve } from "node:path";
import nodeProcess from "node:process";

const goEnvironment = () => ({ ...nodeProcess.env, GOWORK: "off", GOFLAGS: `${nodeProcess.env.GOFLAGS ? `${nodeProcess.env.GOFLAGS} ` : ""}-mod=readonly` });

function assertCandidateSource(directory: string, root: string, client: string, message: string) {
  for (const [module, path] of [
    ["github.com/grafana/ai-sdk", root],
    ["github.com/grafana/ai-sdk/providers/grafana", client],
  ] as const) {
    const selected = JSON.parse(execFileSync("go", ["list", "-m", "-json", module], { cwd: directory, env: goEnvironment(), encoding: "utf8" }));
    assert.equal(realpathSync(selected.Dir), path, message);
  }
}

function anchorReplacements(copy: string, original: string) {
  const manifest = JSON.parse(execFileSync("go", ["mod", "edit", "-json"], { cwd: copy, env: goEnvironment(), encoding: "utf8" })) as { Replace?: Array<{ Old: { Path: string }; New: { Path: string } }> };
  const replacements = (manifest.Replace ?? []).filter(({ New }) => New.Path.startsWith("."))
    .map(({ Old, New }) => `-replace=${Old.Path}=${resolve(original, New.Path)}`);
  if (replacements.length > 0) execFileSync("go", ["mod", "edit", ...replacements], { cwd: copy, env: goEnvironment() });
}

export function buildGoClientCapture(directory: string, sourceDirectory = resolve(import.meta.dirname, "../../../providers/grafana")): string {
  sourceDirectory = realpathSync(sourceDirectory);
  directory = realpathSync(directory);
  const root = realpathSync(resolve(import.meta.dirname, "../../.."));
  const rootClient = join(root, "providers/grafana");
  if (sourceDirectory !== rootClient) anchorReplacements(sourceDirectory, rootClient);
  assertCandidateSource(sourceDirectory, root, sourceDirectory, "Go client capture must select candidate source");
  const binary = join(directory, "go-client-capture");
  execFileSync("go", ["build", "-o", binary, "./internal/capture"], { cwd: sourceDirectory, env: goEnvironment(), stdio: "pipe" });
  return binary;
}

export function buildGoStreamTextCapture(directory: string): string {
  const root = realpathSync(resolve(import.meta.dirname, "../../.."));
  const client = join(root, "providers/grafana");
  assertCandidateSource(client, root, client, "high-level probe must use the changed local source");
  const binary = join(directory, "go-stream-text-capture");
  execFileSync("go", ["build", "-o", binary, "./internal/capture"], { cwd: client, env: goEnvironment(), stdio: "pipe" });
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
