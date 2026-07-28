#!/usr/bin/env tsx

import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parse as parseYaml } from "yaml";

const __dirname = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(__dirname, "..", "..", "..");
const conformanceRoot = join(repoRoot, "test", "conformance");

interface IndexMap {
  [fixtureName: string]: string | null;
}

const providerFixtureRoots: Record<string, string> = {
  anthropic: "packages/anthropic/src/__fixtures__",
  bedrock: "packages/amazon-bedrock/src/__fixtures__",
  openai: "packages/openai/src/responses/__fixtures__",
};

function argValue(name: string): string | undefined {
  const prefix = `${name}=`;
  const match = process.argv.find((arg) => arg.startsWith(prefix));
  return match?.slice(prefix.length);
}

function upstreamRoot(): string | undefined {
  const explicit = argValue("--upstream-root") ?? process.env.AI_SDK_UPSTREAM_ROOT;
  if (explicit) {
    return resolve(explicit.replace(/^~(?=$|\/)/, homedir()));
  }
  const fallback = join(homedir(), "src", "ai");
  return existsSync(fallback) ? fallback : undefined;
}

function listConfigDirs(root: string): string[] {
  const dirs: string[] = [];
  function walk(dir: string): void {
    for (const entry of readdirSync(dir)) {
      const path = join(dir, entry);
      if (!statSync(path).isDirectory()) {
        continue;
      }
      if (existsSync(join(path, "config.yaml"))) {
        dirs.push(path);
        continue;
      }
      walk(path);
    }
  }
  walk(root);
  return dirs.sort();
}

function readIndex(provider: string): IndexMap {
  const path = join(conformanceRoot, provider, "upstream", "INDEX.yaml");
  if (!existsSync(path)) {
    return {};
  }
  return parseYaml(readFileSync(path, "utf8")) as IndexMap;
}

function upstreamStreamingFixtures(root: string, provider: string): string[] {
  const fixtureRoot = join(root, providerFixtureRoots[provider] ?? "");
  if (!fixtureRoot || !existsSync(fixtureRoot)) {
    return [];
  }
  return readdirSync(fixtureRoot)
    .filter((name) => name.endsWith(".chunks.txt"))
    .map((name) => name.replace(/\.chunks\.txt$/, ""))
    .sort();
}

const errors: string[] = [];
const warnings: string[] = [];
const providerCounts = new Map<string, number>();
let expectedObjects = 0;

for (const dir of listConfigDirs(conformanceRoot)) {
  const rel = relative(conformanceRoot, dir);
  const [provider] = rel.split("/");
  providerCounts.set(provider, (providerCounts.get(provider) ?? 0) + 1);

  for (const file of ["expected.jsonl", "expected-requests.jsonl"]) {
    if (!existsSync(join(dir, file))) {
      errors.push(`${rel} is missing ${file}`);
    }
  }
  if (existsSync(join(dir, "expected-object.json"))) {
    expectedObjects++;
  }
}

const root = upstreamRoot();
for (const provider of Object.keys(providerFixtureRoots)) {
  const index = readIndex(provider);
  const listed = new Set(Object.keys(index));
  for (const [fixture, local] of Object.entries(index)) {
    if (local === null) {
      continue;
    }
    const dir = join(conformanceRoot, provider, "upstream", local.split("/")[0]);
    if (!existsSync(join(dir, "config.yaml"))) {
      errors.push(`${provider}/upstream INDEX maps ${fixture} to ${local}, but ${relative(conformanceRoot, dir)} has no config.yaml`);
    }
  }

  if (!root) {
    warnings.push("local upstream clone not found; skipped upstream fixture index drift check");
    continue;
  }
  for (const fixture of upstreamStreamingFixtures(root, provider)) {
    if (!listed.has(fixture)) {
      errors.push(`${provider}/upstream INDEX is missing streaming fixture ${fixture}`);
    }
  }
}

for (const [provider, count] of [...providerCounts.entries()].sort()) {
  console.log(`parity coverage: ${provider} fixtures=${count}`);
}
console.log(`parity coverage: structured-output fixtures=${expectedObjects}`);

for (const warning of warnings) {
  console.warn(`parity coverage: warning: ${warning}`);
}
if (errors.length > 0) {
  for (const error of errors) {
    console.error(`parity coverage: ${error}`);
  }
  process.exitCode = 1;
} else {
  console.log("parity coverage: inventory checks passed");
}
