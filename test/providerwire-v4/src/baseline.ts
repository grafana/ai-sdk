import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { parse as parseYaml } from "yaml";

const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const testRoot = dirname(packageRoot);
export const baselinePath = join(testRoot, "conformance", "upstream.yaml");
export const packageManifestPath = join(packageRoot, "package.json");

export const trackedPackages = [
  "ai",
  "@ai-sdk/gateway",
  "@ai-sdk/provider",
  "@ai-sdk/provider-utils",
] as const;

export type TrackedPackage = (typeof trackedPackages)[number];

export interface RegisteredBaseline {
  upstream?: { repository?: string; commit?: string };
  packages?: Record<string, string>;
}

export interface PackageManifest {
  dependencies?: Record<string, string>;
}

export interface BaselineMetadata {
  commit: string;
  packages: Record<TrackedPackage, string>;
}

export function readRegisteredBaseline(): RegisteredBaseline {
  return parseYaml(readFileSync(baselinePath, "utf8")) as RegisteredBaseline;
}

export function readPackageManifest(): PackageManifest {
  return JSON.parse(readFileSync(packageManifestPath, "utf8")) as PackageManifest;
}

export function baselineMetadata(baseline = readRegisteredBaseline()): BaselineMetadata {
  const commit = baseline.upstream?.commit;
  if (!commit) {
    throw new Error("registered baseline must declare upstream.commit");
  }
  const packages = Object.fromEntries(
    trackedPackages.map((packageName) => {
      const version = baseline.packages?.[packageName];
      if (!version) {
        throw new Error(`registered baseline must declare ${packageName}`);
      }
      return [packageName, version];
    }),
  ) as Record<TrackedPackage, string>;
  return { commit, packages };
}

export function validateBaselineMetadata(
  label: string,
  actual: BaselineMetadata,
  expected = baselineMetadata(),
): string[] {
  const errors: string[] = [];
  if (actual.commit !== expected.commit) {
    errors.push(`${label} commit ${actual.commit} does not match baseline ${expected.commit}`);
  }
  for (const packageName of trackedPackages) {
    if (actual.packages[packageName] !== expected.packages[packageName]) {
      errors.push(
        `${label} ${packageName} version ${actual.packages[packageName]} does not match baseline ${expected.packages[packageName]}`,
      );
    }
  }
  return errors;
}

export function validateWorkspacePackagePins(
  baseline = readRegisteredBaseline(),
  manifest = readPackageManifest(),
): string[] {
  const errors: string[] = [];
  for (const packageName of trackedPackages) {
    const expected = baseline.packages?.[packageName];
    const declared = manifest.dependencies?.[packageName];
    if (!expected || declared !== expected) {
      errors.push(
        `${packageName} workspace version ${declared} does not match baseline ${expected}`,
      );
    }
  }
  return errors;
}
