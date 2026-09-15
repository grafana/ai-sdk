import { execFileSync, spawn } from "node:child_process";
import { join, resolve } from "node:path";
import nodeProcess from "node:process";

export function buildGoClientCapture(directory: string, sourceDirectory = resolve(import.meta.dirname, "../../../providers/grafana")): string {
  const binary = join(directory, "go-client-capture");
  execFileSync("go", ["build", "-o", binary, "./internal/capture"], {
    cwd: sourceDirectory,
    env: { ...nodeProcess.env, GOWORK: "off", GOFLAGS: "-mod=readonly" },
    stdio: "pipe",
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
