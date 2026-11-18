#!/usr/bin/env tsx

import { execFileSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { parse as parseYaml } from "yaml";
import { listProviderInputFiles } from "./fixture-inventory.mts";

const __dirname = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(__dirname, "..", "..", "..");
const conformanceRoot = join(repoRoot, "test", "conformance");

interface IndexMap {
  [fixtureName: string]: string | null;
}

interface Baseline {
  packages?: Record<string, string>;
}

const providerSources = {
  anthropic: { directory: "anthropic", fixtures: "src/__fixtures__" },
  bedrock: { directory: "amazon-bedrock", fixtures: "src/__fixtures__" },
  openai: { directory: "openai", fixtures: "src/responses/__fixtures__" },
  "openai-compatible": { directory: "openai-compatible", fixtures: "src/chat/__fixtures__" },
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

export function providerFixtureReleases(conformanceRoot: string) {
  const baseline = parseYaml(
    readFileSync(join(conformanceRoot, "upstream.yaml"), "utf8"),
  ) as Baseline;
  return Object.entries(providerSources).map(([provider, { directory, fixtures }]) => {
    const packageName = `@ai-sdk/${directory}`;
    const version = baseline?.packages?.[packageName];
    if (typeof version !== "string" || !version) {
      throw new Error(`test/conformance/upstream.yaml is missing a version for ${packageName}`);
    }
    return {
      provider,
      packageName,
      version,
      ref: `refs/tags/${packageName}@${version}`,
      manifestPath: `packages/${directory}/package.json`,
      fixtureRoot: `packages/${directory}/${fixtures}`,
    };
  });
}

function git(root: string, args: string[]): Buffer {
  return execFileSync("git", ["--no-replace-objects", "-C", root, ...args], {
    maxBuffer: 64 * 1024 * 1024,
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function verifyReleaseCommit(root: string, release: ReturnType<typeof providerFixtureReleases>[number]): string {
  const { packageName, version, ref, manifestPath } = release;
  let commit: string;
  try {
    const tag = git(root, ["show-ref", "--verify", "--hash", ref]).toString("utf8").trim();
    commit = git(root, ["rev-parse", "--verify", `${tag}^{commit}`]).toString("utf8").trim();
  } catch (cause) {
    throw new Error(`missing or unreadable release tag ${ref} in ${root}; fetch the exact tag into the upstream checkout`, { cause });
  }

  let manifest: { name?: string; version?: string };
  try {
    manifest = JSON.parse(git(root, ["show", `${commit}:${manifestPath}`]).toString("utf8"));
  } catch (cause) {
    throw new Error(`cannot read ${manifestPath} at ${ref} (${commit}) to verify ${packageName}@${version}`, { cause });
  }
  if (manifest?.name !== packageName || manifest.version !== version) {
    throw new Error(`${manifestPath} at ${ref} (${commit}): expected ${packageName}@${version}, found ${manifest?.name ?? "(no name)"}@${manifest?.version ?? "(no version)"}`);
  }
  return commit;
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

function readIndex(conformanceRoot: string, provider: string): IndexMap {
  const path = join(conformanceRoot, provider, "upstream", "INDEX.yaml");
  if (!existsSync(path)) {
    return {};
  }
  return parseYaml(readFileSync(path, "utf8")) as IndexMap;
}

function upstreamFixtureNames(
  root: string,
  commit: string,
  fixtureRoot: string,
): string[] {
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
  fixtureRoot: string,
  fixture: string,
): string | undefined {
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
  conformanceRoot: string,
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

export function checkProviderInventories(conformanceRoot: string, root: string): string[] {
  const releases = providerFixtureReleases(conformanceRoot).map(release => ({
    ...release,
    commit: verifyReleaseCommit(root, release),
  }));
  const errors: string[] = [];

  for (const { provider, fixtureRoot, commit } of releases) {
    const index = readIndex(conformanceRoot, provider);
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

    for (const fixture of upstreamFixtureNames(root, commit, fixtureRoot)) {
      if (!listed.has(fixture)) {
        errors.push(`${provider}/upstream INDEX is missing pinned fixture ${fixture}`);
      }
    }

    const expectedLocalInputs = new Set<string>();
    for (const [fixture, local] of Object.entries(index)) {
      const sourcePath = upstreamFixturePath(root, commit, fixtureRoot, fixture);
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
      const localPath = localFixturePath(conformanceRoot, provider, fixture, local, sourcePath, index);
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

  return errors;
}

function main(): void {
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

  errors.push(...checkProviderInventories(conformanceRoot, upstreamRoot()));
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
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    main();
  } catch (err) {
    console.error(`parity coverage: ${err}`);
    process.exitCode = 1;
  }
}
