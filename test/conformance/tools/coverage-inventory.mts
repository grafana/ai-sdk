#!/usr/bin/env tsx

import { execFileSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parse as parseYaml } from "yaml";
import { listProviderInputFiles } from "./fixture-inventory.mts";

const __dirname = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(__dirname, "..", "..", "..");
const conformanceRoot = join(repoRoot, "test", "conformance");

interface IndexMap {
  [fixtureName: string]: string | null;
}

interface Baseline {
  upstream: {
    commit: string;
  };
}

const providerFixtureRoots: Record<string, string> = {
  anthropic: "packages/anthropic/src/__fixtures__",
  bedrock: "packages/amazon-bedrock/src/__fixtures__",
  openai: "packages/openai/src/responses/__fixtures__",
  "openai-compatible": "packages/openai-compatible/src/chat/__fixtures__",
};

function argValue(name: string): string | undefined {
  const prefix = `${name}=`;
  const match = process.argv.find((arg) => arg.startsWith(prefix));
  return match?.slice(prefix.length);
}

function upstreamRoot(): string {
  const explicit = argValue("--upstream-root") ?? process.env.AI_SDK_UPSTREAM_ROOT;
  const root = explicit
    ? resolve(explicit.replace(/^~(?=$|\/)/, homedir()))
    : join(homedir(), "src", "ai");
  if (!existsSync(root)) {
    throw new Error(`pinned upstream checkout not found at ${root}`);
  }
  return root;
}

function baselineCommit(): string {
  const manifest = parseYaml(
    readFileSync(join(conformanceRoot, "upstream.yaml"), "utf8"),
  ) as Baseline;
  if (!manifest.upstream?.commit) {
    throw new Error("test/conformance/upstream.yaml is missing upstream.commit");
  }
  return manifest.upstream.commit;
}

function git(root: string, args: string[]): Buffer {
  return execFileSync("git", ["-C", root, ...args], {
    maxBuffer: 64 * 1024 * 1024,
    stdio: ["ignore", "pipe", "pipe"],
  });
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

function upstreamFixtureNames(
  root: string,
  commit: string,
  provider: string,
): string[] {
  const fixtureRoot = providerFixtureRoots[provider];
  const files = git(root, [
    "ls-tree",
    "-r",
    "--name-only",
    commit,
    "--",
    fixtureRoot,
  ]).toString("utf8").split("\n");
  const streaming = new Set<string>();
  const unary = new Set<string>();
  for (const file of files) {
    const name = file.slice(fixtureRoot.length + 1);
    if (!name || name.includes("/")) continue;
    if (name.endsWith(".chunks.txt")) {
      streaming.add(name.slice(0, -".chunks.txt".length));
    } else if (name.endsWith(".json")) {
      unary.add(name.slice(0, -".json".length));
    }
  }
  return [
    ...streaming,
    ...[...unary].map(name => streaming.has(name) ? `${name}.json` : name),
  ].sort();
}

function upstreamFixturePath(
  root: string,
  commit: string,
  provider: string,
  fixture: string,
): string | undefined {
  const fixtureRoot = providerFixtureRoots[provider];
  const suffixes = fixture.endsWith(".chunks.txt") || fixture.endsWith(".json")
    ? [""]
    : [".chunks.txt", ".json"];
  for (const suffix of suffixes) {
    const path = `${fixtureRoot}/${fixture}${suffix}`;
    try {
      git(root, ["cat-file", "-e", `${commit}:${path}`]);
      return path;
    } catch {
      continue;
    }
  }
  return undefined;
}

function upstreamFixtureContent(root: string, commit: string, path: string): Buffer {
  return git(root, ["show", `${commit}:${path}`]);
}

function localFixturePath(
  provider: string,
  fixture: string,
  local: string,
  sourcePath: string,
  index: IndexMap,
): string {
  const localRoot = join(conformanceRoot, provider, "upstream");
  if (local.includes("/")) {
    return join(localRoot, local);
  }
  if (sourcePath.endsWith(".json")) {
    return join(localRoot, local, "input.response.json");
  }

  const mappingsToDirectory = Object.values(index).filter(value => value === local).length;
  const numbered = /\.(\d+)$/.exec(fixture);
  const inputName = mappingsToDirectory > 1 && numbered
    ? `input-${numbered[1]}.chunks.txt`
    : "input.chunks.txt";
  return join(localRoot, local, inputName);
}

const errors: string[] = [];
const providerCounts = new Map<string, number>();
let expectedObjects = 0;

for (const dir of listConfigDirs(conformanceRoot)) {
  const rel = relative(conformanceRoot, dir);
  const [provider] = rel.split("/");
  providerCounts.set(provider, (providerCounts.get(provider) ?? 0) + 1);

  const config = parseYaml(readFileSync(join(dir, "config.yaml"), "utf8")) as {
    operation?: string;
  };
  const expectedResult = config.operation === "generate"
    ? "expected-generate.json"
    : "expected.jsonl";
  for (const file of [expectedResult, "expected-requests.jsonl"]) {
    if (!existsSync(join(dir, file))) {
      errors.push(`${rel} is missing ${file}`);
    }
  }
  if (existsSync(join(dir, "expected-object.json"))) {
    expectedObjects++;
  }
}

let root = "";
let commit = "";
try {
  root = upstreamRoot();
  commit = baselineCommit();
  git(root, ["cat-file", "-e", `${commit}^{commit}`]);
} catch (err) {
  console.error(`parity coverage: ${err}`);
  process.exit(1);
}

for (const provider of Object.keys(providerFixtureRoots)) {
  const index = readIndex(provider);
  const listed = new Set(Object.keys(index));
  const mappedDirectories = new Set<string>();
  for (const [fixture, local] of Object.entries(index)) {
    if (local === null) continue;
    const directory = local.split("/")[0];
    mappedDirectories.add(directory);
    const dir = join(conformanceRoot, provider, "upstream", directory);
    if (!existsSync(join(dir, "config.yaml"))) {
      errors.push(`${provider}/upstream INDEX maps ${fixture} to ${local}, but ${relative(conformanceRoot, dir)} has no config.yaml`);
    }
  }

  const localUpstreamRoot = join(conformanceRoot, provider, "upstream");
  if (existsSync(localUpstreamRoot)) {
    for (const entry of readdirSync(localUpstreamRoot, { withFileTypes: true })) {
      if (
        entry.isDirectory() &&
        existsSync(join(localUpstreamRoot, entry.name, "config.yaml")) &&
        !mappedDirectories.has(entry.name)
      ) {
        errors.push(`${provider}/upstream/${entry.name} is not mapped by INDEX.yaml`);
      }
    }
  }

  for (const fixture of upstreamFixtureNames(root, commit, provider)) {
    if (!listed.has(fixture)) {
      errors.push(`${provider}/upstream INDEX is missing pinned fixture ${fixture}`);
    }
  }

  const expectedLocalInputs = new Set<string>();
  for (const [fixture, local] of Object.entries(index)) {
    const sourcePath = upstreamFixturePath(root, commit, provider, fixture);
    if (!sourcePath) {
      errors.push(`${provider}/upstream INDEX references nonexistent pinned fixture ${fixture}`);
      continue;
    }
    if (local === null) {
      if (sourcePath.endsWith(".chunks.txt")) {
        errors.push(`${provider}/upstream INDEX leaves streaming fixture ${fixture} unimported`);
      }
      continue;
    }
    const localPath = localFixturePath(provider, fixture, local, sourcePath, index);
    expectedLocalInputs.add(localPath);
    if (!existsSync(localPath)) {
      errors.push(`${provider}/upstream INDEX maps ${fixture} to missing ${relative(conformanceRoot, localPath)}`);
      continue;
    }
    const source = upstreamFixtureContent(root, commit, sourcePath);
    const imported = readFileSync(localPath);
    if (!source.equals(imported)) {
      errors.push(`${provider}/upstream ${fixture} is not byte-identical to ${sourcePath} at ${commit}`);
    }
  }

  if (existsSync(localUpstreamRoot)) {
    for (const inputPath of listProviderInputFiles(localUpstreamRoot)) {
      if (!expectedLocalInputs.has(inputPath)) {
        errors.push(`${provider}/upstream has unindexed provider input ${relative(conformanceRoot, inputPath)}`);
      }
    }
  }
}

for (const [provider, count] of [...providerCounts.entries()].sort()) {
  console.log(`parity coverage: ${provider} fixtures=${count}`);
}
console.log(`parity coverage: structured-output fixtures=${expectedObjects}`);

if (errors.length > 0) {
  for (const error of errors) {
    console.error(`parity coverage: ${error}`);
  }
  process.exitCode = 1;
} else {
  console.log("parity coverage: inventory checks passed");
}
