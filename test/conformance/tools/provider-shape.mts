#!/usr/bin/env tsx

import { existsSync, readFileSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parse as parseYaml } from "yaml";

const __dirname = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(__dirname, "..", "..", "..");

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

  const baseline = parseYaml(
    readFileSync(join(repoRoot, "test", "conformance", "upstream.yaml"), "utf8"),
  ) as { packages?: Record<string, unknown> };
  const providerVersion = baseline.packages?.["@ai-sdk/provider"];
  if (typeof providerVersion !== "string") {
    throw new Error("upstream baseline must declare @ai-sdk/provider");
  }

  const pinnedProvider = join(
    repoRoot,
    "test",
    "node_modules",
    ".pnpm",
    `@ai-sdk+provider@${providerVersion}`,
    "node_modules",
    "@ai-sdk",
    "provider",
    "src",
  );
  return existsSync(pinnedProvider) ? pinnedProvider : undefined;
}

function quotedValues(source: string, regex: RegExp): Set<string> {
  const values = new Set<string>();
  for (const match of source.matchAll(regex)) {
    values.add(match[1]);
  }
  return values;
}

function goConstValues(path: string, typeName: string): Set<string> {
  const source = readFileSync(path, "utf8");
  return quotedValues(source, new RegExp(`${typeName}\\s*=\\s*"([^"]+)"`, "g"));
}

function upstreamContentValues(upstreamV4: string): Set<string> {
  const source = readFileSync(join(upstreamV4, "language-model-v4-content.ts"), "utf8");
  const values = new Set<string>();
  for (const match of source.matchAll(/from\s+'\.\/([^']+)'/g)) {
    const partSource = readFileSync(join(upstreamV4, `${match[1]}.ts`), "utf8");
    for (const value of quotedValues(partSource, /type:\s*'([^']+)'/g)) {
      if (value === "data" || value === "url") {
        continue;
      }
      values.add(value);
    }
  }
  return values;
}

const root = upstreamRoot();
if (!root) {
  console.warn("provider shape: registered provider package source not found; skipped provider V4 drift report");
  process.exit(0);
}

const upstreamV4 = root.endsWith(join("@ai-sdk", "provider", "src"))
  ? join(root, "language-model", "v4")
  : join(root, "packages", "provider", "src", "language-model", "v4");
const upstreamSharedV4 = root.endsWith(join("@ai-sdk", "provider", "src"))
  ? join(root, "shared", "v4")
  : join(root, "packages", "provider", "src", "shared", "v4");
const checks = [
  {
    name: "stream part",
    upstream: quotedValues(readFileSync(join(upstreamV4, "language-model-v4-stream-part.ts"), "utf8"), /type:\s*'([^']+)'/g),
    local: goConstValues(join(repoRoot, "provider", "stream_part.go"), "StreamPartType"),
  },
  {
    name: "content part",
    upstream: upstreamContentValues(upstreamV4),
    local: goConstValues(join(repoRoot, "provider", "language_model.go"), "GenerateContentType"),
  },
  {
    name: "finish reason",
    upstream: quotedValues(readFileSync(join(upstreamV4, "language-model-v4-finish-reason.ts"), "utf8"), /'([^']+)'/g),
    local: goConstValues(join(repoRoot, "provider", "types.go"), "UnifiedFinishReason"),
  },
  {
    name: "tool result output",
    upstream: new Set(["text", "json", "execution-denied", "error-text", "error-json", "content"]),
    local: goConstValues(join(repoRoot, "provider", "types.go"), "ToolResultOutputType"),
  },
  {
    name: "warning",
    upstream: quotedValues(readFileSync(join(upstreamSharedV4, "shared-v4-warning.ts"), "utf8"), /type:\s*'([^']+)'/g),
    local: goConstValues(join(repoRoot, "provider", "types.go"), "WarningType"),
  },
];

for (const check of checks) {
  const missingValues = [...check.upstream]
    .filter((value) => !check.local.has(value))
    .sort();
  if (missingValues.length === 0) {
    console.log(`provider shape: ${check.name} values match`);
    continue;
  }
  console.warn(`provider shape: missing ${check.name} values: ${missingValues.join(", ")}`);
}
