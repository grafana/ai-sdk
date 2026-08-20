import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  baselineMetadata,
  readPackageManifest,
  readRegisteredBaseline,
  trackedPackages,
  validateBaselineMetadata,
  validateWorkspacePackagePins,
  type PackageManifest,
  type RegisteredBaseline,
  type TrackedPackage,
} from "./baseline.ts";

const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
export const sourceEquivalencePath = join(packageRoot, "artifacts", "source-equivalence.json");

export interface SourceInput {
  package: string;
  installedPath: string;
  upstreamPath: string;
}

const relevantSources = {
  provider: [
    "errors/ai-sdk-error.ts",
    "errors/api-call-error.ts",
    "errors/empty-response-body-error.ts",
    "errors/invalid-response-data-error.ts",
    "errors/json-parse-error.ts",
    "errors/type-validation-error.ts",
    "json-value/index.ts",
    "json-value/json-value.ts",
    "language-model/v4/index.ts",
    "language-model/v4/language-model-v4-call-options.ts",
    "language-model/v4/language-model-v4-content.ts",
    "language-model/v4/language-model-v4-custom-content.ts",
    "language-model/v4/language-model-v4-file.ts",
    "language-model/v4/language-model-v4-finish-reason.ts",
    "language-model/v4/language-model-v4-function-tool.ts",
    "language-model/v4/language-model-v4-generate-result.ts",
    "language-model/v4/language-model-v4-prompt.ts",
    "language-model/v4/language-model-v4-provider-tool.ts",
    "language-model/v4/language-model-v4-reasoning-file.ts",
    "language-model/v4/language-model-v4-reasoning.ts",
    "language-model/v4/language-model-v4-response-metadata.ts",
    "language-model/v4/language-model-v4-source.ts",
    "language-model/v4/language-model-v4-stream-part.ts",
    "language-model/v4/language-model-v4-stream-result.ts",
    "language-model/v4/language-model-v4-text.ts",
    "language-model/v4/language-model-v4-tool-approval-request.ts",
    "language-model/v4/language-model-v4-tool-call.ts",
    "language-model/v4/language-model-v4-tool-choice.ts",
    "language-model/v4/language-model-v4-tool-result.ts",
    "language-model/v4/language-model-v4-usage.ts",
    "language-model/v4/language-model-v4.ts",
    "shared/v4/index.ts",
    "shared/v4/shared-v4-file-data.ts",
    "shared/v4/shared-v4-headers.ts",
    "shared/v4/shared-v4-provider-metadata.ts",
    "shared/v4/shared-v4-provider-options.ts",
    "shared/v4/shared-v4-provider-reference.ts",
    "shared/v4/shared-v4-warning.ts",
  ],
  gateway: [
    "errors/as-gateway-error.ts",
    "errors/create-gateway-error.ts",
    "errors/extract-api-call-response.ts",
    "errors/gateway-authentication-error.ts",
    "errors/gateway-error.ts",
    "errors/gateway-failed-dependency-error.ts",
    "errors/gateway-forbidden-error.ts",
    "errors/gateway-internal-server-error.ts",
    "errors/gateway-invalid-request-error.ts",
    "errors/gateway-model-not-found-error.ts",
    "errors/gateway-rate-limit-error.ts",
    "errors/gateway-response-error.ts",
    "errors/gateway-timeout-error.ts",
    "errors/index.ts",
    "errors/parse-auth-method.ts",
    "gateway-config.ts",
    "gateway-headers.ts",
    "gateway-language-model-settings.ts",
    "gateway-language-model.ts",
    "gateway-provider.ts",
    "vercel-environment.ts",
    "version.ts",
    "zod.ts",
  ],
  "provider-utils": [
    "cancel-response-body.ts",
    "combine-headers.ts",
    "download-error.ts",
    "extract-response-headers.ts",
    "fetch-function.ts",
    "get-error-message.ts",
    "get-runtime-environment-user-agent.ts",
    "handle-fetch-error.ts",
    "is-abort-error.ts",
    "is-json-serializable.ts",
    "load-optional-setting.ts",
    "maybe-promise-like.ts",
    "normalize-headers.ts",
    "parse-json-event-stream.ts",
    "parse-json.ts",
    "post-to-api.ts",
    "read-response-with-size-limit.ts",
    "resolve.ts",
    "response-handler.ts",
    "schema.ts",
    "secure-json-parse.ts",
    "serialization-error.ts",
    "serialize-model-options.ts",
    "validate-types.ts",
    "version.ts",
    "with-user-agent-suffix.ts",
    "without-trailing-slash.ts",
  ],
} as const;

export const sourceInputs: SourceInput[] = Object.entries(relevantSources).flatMap(
  ([packageName, paths]) =>
    paths.map((path) => ({
      package: `@ai-sdk/${packageName}`,
      installedPath: `src/${path}`,
      upstreamPath: `packages/${packageName}/src/${path}`,
    })),
);

export interface SourceEquivalenceEvidence {
  schemaVersion: 1;
  upstream: { repository: string; commit: string };
  packages: Record<string, string>;
  files: Array<SourceInput & { sha256: string }>;
}

function sha256(value: Buffer | string): string {
  return createHash("sha256").update(value).digest("hex");
}

function readInputs(): { baseline: RegisteredBaseline; packageManifest: PackageManifest } {
  return {
    baseline: readRegisteredBaseline(),
    packageManifest: readPackageManifest(),
  };
}

function installedBytes(input: SourceInput): Buffer {
  return readFileSync(join(packageRoot, "node_modules", input.package, input.installedPath));
}

export function validateSourceEquivalenceEvidence(
  evidence: SourceEquivalenceEvidence,
  baseline: RegisteredBaseline,
  packageManifest: PackageManifest,
  readInstalled: (input: SourceInput) => Buffer = installedBytes,
): string[] {
  const errors: string[] = [];
  if (evidence.schemaVersion !== 1) {
    errors.push(`source equivalence schema version ${evidence.schemaVersion} is not supported`);
  }
  const packageKeys = Object.keys(evidence.packages).sort();
  const expectedPackageKeys = [...trackedPackages].sort();
  if (packageKeys.join("\n") !== expectedPackageKeys.join("\n")) {
    errors.push(
      `source equivalence package keys ${packageKeys.join(", ")} do not match ${expectedPackageKeys.join(", ")}`,
    );
  }
  const expected = baselineMetadata(baseline);
  const evidenceMetadata = {
    commit: evidence.upstream.commit,
    packages: Object.fromEntries(
      trackedPackages.map((packageName) => [packageName, evidence.packages[packageName]]),
    ) as Record<TrackedPackage, string>,
  };
  errors.push(...validateBaselineMetadata("source equivalence", evidenceMetadata, expected));
  errors.push(...validateWorkspacePackagePins(baseline, packageManifest));
  if (evidence.upstream.repository !== baseline.upstream?.repository) {
    errors.push(
      `source equivalence repository ${evidence.upstream.repository} does not match baseline ${baseline.upstream?.repository}`,
    );
  }

  const expectedKeys = new Set(sourceInputs.map(sourceKey));
  const actualKeys = new Set<string>();
  for (const file of evidence.files) {
    const key = sourceKey(file);
    if (actualKeys.has(key)) {
      errors.push(`duplicate source-equivalence entry ${key}`);
      continue;
    }
    actualKeys.add(key);
    if (!expectedKeys.has(key)) {
      errors.push(`unexpected source-equivalence entry ${key}`);
      continue;
    }
    try {
      const actualHash = sha256(readInstalled(file));
      if (actualHash !== file.sha256) {
        errors.push(`${key} installed hash ${actualHash} does not match evidence ${file.sha256}`);
      }
    } catch (error) {
      errors.push(`${key} cannot be read: ${error instanceof Error ? error.message : String(error)}`);
    }
  }
  for (const key of expectedKeys) {
    if (!actualKeys.has(key)) {
      errors.push(`missing source-equivalence entry ${key}`);
    }
  }
  return errors;
}

function sourceKey(input: SourceInput): string {
  return `${input.package}:${input.installedPath}:${input.upstreamPath}`;
}

function gitShow(repositoryDir: string, commit: string, path: string): Buffer {
  return execFileSync("git", ["-C", repositoryDir, "show", `${commit}:${path}`], {
    encoding: "buffer",
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function withUpstreamRepository<T>(
  baseline: RegisteredBaseline,
  action: (repositoryDir: string) => T,
): T {
  const configured = process.env.AI_SDK_UPSTREAM_DIR;
  if (configured) {
    return action(configured);
  }
  const repository = baseline.upstream?.repository;
  const commit = baseline.upstream?.commit;
  if (!repository || !commit) {
    throw new Error("baseline must declare upstream.repository and upstream.commit");
  }
  const temporaryDirectory = mkdtempSync(join(tmpdir(), "providerwire-v4-upstream-"));
  try {
    execFileSync("git", ["clone", "--filter=blob:none", "--no-checkout", repository, temporaryDirectory], {
      stdio: "inherit",
    });
    execFileSync("git", ["-C", temporaryDirectory, "fetch", "origin", commit], { stdio: "inherit" });
    return action(temporaryDirectory);
  } finally {
    rmSync(temporaryDirectory, { recursive: true, force: true });
  }
}

export function buildSourceEquivalenceEvidence(): SourceEquivalenceEvidence {
  const { baseline, packageManifest } = readInputs();
  const repository = baseline.upstream?.repository;
  const commit = baseline.upstream?.commit;
  if (!repository || !commit) {
    throw new Error("baseline must declare upstream.repository and upstream.commit");
  }

  return withUpstreamRepository(baseline, (repositoryDir) => {
    const files = sourceInputs.map((input) => {
      const installed = installedBytes(input);
      const upstream = gitShow(repositoryDir, commit, input.upstreamPath);
      if (!installed.equals(upstream)) {
        throw new Error(
          `${input.package}/${input.installedPath} differs from ${commit}:${input.upstreamPath}`,
        );
      }
      return { ...input, sha256: sha256(installed) };
    });
    const pinErrors = validateWorkspacePackagePins(baseline, packageManifest);
    if (pinErrors.length > 0) {
      throw new Error(pinErrors.join("\n"));
    }
    const packages = Object.fromEntries(
      trackedPackages.map((packageName) => [packageName, packageManifest.dependencies?.[packageName]]),
    ) as Record<TrackedPackage, string>;
    return {
      schemaVersion: 1,
      upstream: { repository, commit },
      packages,
      files,
    };
  });
}

export function verifyCommittedSourceEquivalence(
  committedPath = sourceEquivalencePath,
): void {
  const { baseline, packageManifest } = readInputs();
  const evidence = JSON.parse(readFileSync(committedPath, "utf8")) as SourceEquivalenceEvidence;
  const errors = validateSourceEquivalenceEvidence(evidence, baseline, packageManifest);
  if (errors.length > 0) {
    throw new Error(errors.join("\n"));
  }
}

function main(): void {
  if (process.argv.includes("--write")) {
    const evidence = buildSourceEquivalenceEvidence();
    writeFileSync(sourceEquivalencePath, `${JSON.stringify(evidence, null, 2)}\n`);
    console.log(`wrote ${sourceEquivalencePath}`);
    return;
  }
  verifyCommittedSourceEquivalence();
  console.log("ProviderWire V4 source-equivalence evidence matches installed package inputs");
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main();
}
