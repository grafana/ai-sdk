#!/usr/bin/env tsx

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { parse as parseYaml } from "yaml";

const __dirname = dirname(fileURLToPath(import.meta.url));

export interface BaselineManifest {
  packages?: Record<string, unknown>;
}

export interface PackageManifest {
  dependencies?: Record<string, string>;
  devDependencies?: Record<string, string>;
}

export const providerWireRequiredPackages = [
  "@ai-sdk/gateway",
  "@ai-sdk/provider",
  "@ai-sdk/provider-utils",
] as const;

export function validateBaseline(
  baseline: BaselineManifest,
  packageManifest: PackageManifest,
  packageLabel = "package.json",
  requiredPackages: readonly string[] = [],
): string[] {
  const errors: string[] = [];
  const baselinePackages = baseline.packages ?? {};
  const packageVersions = {
    ...(packageManifest.dependencies ?? {}),
    ...(packageManifest.devDependencies ?? {}),
  };

  for (const [name, version] of Object.entries(packageVersions)) {
    if (name !== "ai" && !name.startsWith("@ai-sdk/")) {
      continue;
    }
    const baselineVersion = baselinePackages[name];
    if (typeof baselineVersion !== "string") {
      errors.push(`${packageLabel} dependency ${name}@${version} is missing from baseline manifest`);
      continue;
    }
    if (baselineVersion !== version) {
      errors.push(`${packageLabel} dependency ${name} pins ${version}, but baseline declares ${baselineVersion}`);
    }
  }

  for (const name of requiredPackages) {
    if (packageVersions[name] !== undefined) {
      continue;
    }
    const baselineVersion = baselinePackages[name];
    errors.push(
      typeof baselineVersion === "string"
        ? `${packageLabel} must declare dependency ${name}@${baselineVersion}`
        : `${packageLabel} required dependency ${name} is missing from baseline manifest`,
    );
  }

  return errors;
}

export function validateBaselineFiles(manifestPath: string, packagePaths: string[]): string[] {
  const baseline = parseYaml(readFileSync(manifestPath, "utf8")) as BaselineManifest;
  const errors: string[] = [];

  for (const packagePath of packagePaths) {
    const packageManifest = JSON.parse(readFileSync(packagePath, "utf8")) as PackageManifest;
    const packageLabel = relative(process.cwd(), packagePath);
    const requiredPackages = packagePath.replaceAll("\\", "/").endsWith("/providerwire-v4/package.json")
      ? providerWireRequiredPackages
      : [];
    errors.push(...validateBaseline(baseline, packageManifest, packageLabel, requiredPackages));
  }

  return errors;
}

export function defaultPackagePaths(baseDirectory = __dirname): string[] {
  return [
    join(baseDirectory, "package.json"),
    join(baseDirectory, "..", "..", "integration", "package.json"),
    join(baseDirectory, "..", "..", "cli", "package.json"),
    join(baseDirectory, "..", "..", "providerwire-v4", "package.json"),
  ];
}

function argValue(name: string): string | undefined {
  const prefix = `${name}=`;
  const match = process.argv.find((arg) => arg.startsWith(prefix));
  return match?.slice(prefix.length);
}

function main(): void {
  const manifestPath = argValue("--manifest") ?? join(__dirname, "..", "upstream.yaml");
  const packageArgs = process.argv
    .filter((arg) => arg.startsWith("--package="))
    .map((arg) => arg.slice("--package=".length));
  const packagePaths = packageArgs.length > 0 ? packageArgs : defaultPackagePaths();
  const errors = validateBaselineFiles(manifestPath, packagePaths);
  if (errors.length > 0) {
    for (const error of errors) {
      console.error(`parity baseline: ${error}`);
    }
    process.exitCode = 1;
    return;
  }
  console.log("parity baseline: package versions match all parity TypeScript consumers");
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main();
}
