import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { relative, resolve } from "node:path";
import { describe, it } from "node:test";
import { parseAllDocuments, parseDocument } from "yaml";

const root = resolve(import.meta.dirname, "../../..");
const document = parseDocument(readFileSync(resolve(root, ".github/workflows/ci.yml"), "utf8"));
assert.deepEqual(document.errors, []);
const workflow = document.toJS() as { concurrency: { group: string; "cancel-in-progress": string }; on: { push: { branches: string[]; tags?: string[] }; pull_request: unknown }; jobs: Record<string, { if?: string; needs?: string | string[]; permissions?: Record<string, string>; "continue-on-error"?: boolean; steps: Array<{ name?: string; run?: string; uses?: string; with?: { "persist-credentials"?: boolean }; "continue-on-error"?: boolean }> }> };
const jobs = workflow.jobs;
const source = ["ci", "race", "docs-lint", "module-resolution", "parity-baseline", "integration-test", "conformance-test"];
const needs = (job: string) => [jobs[job]!.needs ?? []].flat();
const commands = (job: string) => jobs[job]!.steps.map((step) => step.run ?? "").join("\n");
const dependencies = (job: string) => {
  const seen = new Set<string>();
  const pending = [...needs(job)];
  while (pending.length > 0) {
    const dependency = pending.pop()!;
    assert.ok(jobs[dependency], `missing dependency ${dependency}`);
    if (seen.has(dependency)) continue;
    seen.add(dependency);
    pending.push(...needs(dependency));
  }
  return seen;
};

describe("candidate-source workflow gates", () => {
  it("keeps pnpm packages under the shared lockfile root", () => {
    const workspace = parseDocument(readFileSync(resolve(root, "pnpm-workspace.yaml"), "utf8")).toJS() as { packages: string[] };
    const lockfile = parseAllDocuments(readFileSync(resolve(root, "pnpm-lock.yaml"), "utf8")).at(-1)?.toJS() as { importers: Record<string, unknown> };
    for (const packagePath of workspace.packages) {
      assert.equal(relative(root, resolve(root, packagePath)), packagePath);
      assert.ok(Object.hasOwn(lockfile.importers, packagePath), `${packagePath} missing from root lockfile`);
    }
  });

  it("keeps candidate-source checks blocking without standalone prerequisites", () => {
    for (const job of source) {
      assert.ok(jobs[job], `missing source check ${job}`);
      assert.equal(jobs[job]!.if, job === "ci" ? "${{ always() }}" : undefined, `${job} must run on PR and push`);
      const required = dependencies(job);
      assert.notEqual(jobs[job]!["continue-on-error"], true, `${job} must block on failure`);
      assert.ok(jobs[job]!.steps.every((step) => step["continue-on-error"] !== true));
      for (const path of [job, ...required]) {
        assert.doesNotMatch(commands(path), /verify-module-resolution|verify-published-module|test-candidate-source|test-go-client-standalone/);
      }
    }
    for (const task of ["test-module-policy", "test-ci-workflow", "verify-gateway-workspace", "verify-ai-gateway-boundary", "verify-merged-pins", "verify-sdk-gateway-isolation"]) {
      assert.match(commands("module-resolution"), new RegExp(`mise run ${task}`));
    }
    assert.match(commands("parity-baseline"), /test-providerwire-v4/);
    assert.match(commands("conformance-test"), /test-conformance/);
    assert.match(commands("integration-test"), /test-integration/);
    for (const task of ["fmt-check", "release-check", "vet", "build", "test-short"]) {
      assert.match(commands("build-test"), new RegExp(`mise run ${task}\\b`));
    }
    assert.match(commands("lint"), /mise run lint/);
    assert.match(commands("race"), /mise run test-race/);
  });

  it("keeps the seven required source checks and their workers without image automation", () => {
    assert.deepEqual(Object.keys(jobs).sort(), [...source, "build-test", "lint"].sort());
    assert.deepEqual(workflow.on.push, { branches: ["main"] });
    assert.ok(Object.hasOwn(workflow.on, "pull_request"));
    assert.deepEqual(Object.keys(workflow.on).sort(), ["pull_request", "push"]);
    for (const job of Object.keys(jobs)) {
      dependencies(job);
      assert.notEqual(jobs[job]!.permissions?.packages, "write");
      assert.doesNotMatch(commands(job), /docker (build|buildx|run|login|push)|build-ai-gateway-image|test-ai-gateway-image-source/);
      assert.ok(jobs[job]!.steps.every((step) => !step.uses?.startsWith("docker/")));
    }
  });

  it("cancels superseded PR runs while keeping main runs independent", () => {
    assert.deepEqual(workflow.concurrency, {
      group: "${{ github.workflow }}-${{ github.event.pull_request.number || github.run_id }}",
      "cancel-in-progress": "${{ github.event_name == 'pull_request' }}",
    });
  });

  it("fails the ci gate unless both independent workers succeed", () => {
    assert.deepEqual(needs("ci"), ["build-test", "lint"]);
    assert.deepEqual(needs("build-test"), []);
    assert.deepEqual(needs("lint"), []);
    for (const worker of ["build-test", "lint"]) {
      assert.equal(jobs[worker]!.if, undefined);
      assert.notEqual(jobs[worker]!["continue-on-error"], true);
      assert.ok(jobs[worker]!.steps.every((step) => step["continue-on-error"] !== true));
    }
    const gate = jobs.ci!.steps[0] as { env: Record<string, string> };
    assert.deepEqual(gate.env, {
      BUILD_TEST_RESULT: "${{ needs.build-test.result }}",
      LINT_RESULT: "${{ needs.lint.result }}",
    });
    for (const build of ["success", "failure", "cancelled", "skipped"]) {
      for (const lint of ["success", "failure", "cancelled", "skipped"]) {
        const result = spawnSync("bash", ["-c", commands("ci")], {
          env: { ...process.env, BUILD_TEST_RESULT: build, LINT_RESULT: lint },
        });
        assert.equal(result.status === 0, build === "success" && lint === "success", `${build}/${lint}`);
      }
    }
  });

  it("refreshes isolated build caches for every Go job", () => {
    const cacheDocument = parseDocument(readFileSync(resolve(root, ".github/actions/cache-go/action.yml"), "utf8"));
    assert.deepEqual(cacheDocument.errors, []);
    const cache = cacheDocument.toJS() as { runs: { using: string; steps: Array<{ uses?: string; with?: { key: string; path: string; "restore-keys": string } }> } };
    assert.equal(cache.runs.using, "composite");
    const caches = cache.runs.steps.filter((step) => step.uses?.startsWith("actions/cache@"));
    assert.equal(caches.length, 2);
    assert.notEqual(caches[0]!.with!.path, caches[1]!.with!.path);
    for (const step of caches) {
      assert.ok(step.with!.key.includes("${{ github.job }}"));
      assert.ok(step.with!.key.includes("${{ steps.paths.outputs.version }}"));
    }
    assert.ok(caches[1]!.with!.key.endsWith("-${{ github.sha }}"));
    assert.ok(!caches[1]!.with!["restore-keys"].includes("${{ github.sha }}"));
    for (const job of ["build-test", "lint", "race", "module-resolution", "parity-baseline", "integration-test", "conformance-test"]) {
      const steps = jobs[job]!.steps;
      const cacheIndex = steps.findIndex((step) => step.uses === "./.github/actions/cache-go");
      assert.ok(cacheIndex > steps.findIndex((step) => step.uses?.startsWith("actions/checkout@")), job);
      assert.ok(cacheIndex < steps.findIndex((step) => step.run), job);
    }
  });

  it("preserves the local workspace image recipe independently of CI", () => {
    const dockerfile = readFileSync(resolve(root, "ai-gateway/Dockerfile"), "utf8");
    assert.match(dockerfile, /GOWORK=\/src\/go\.gateway\.work/);
    assert.doesNotMatch(dockerfile, /GOWORK=off/);
    assert.match(readFileSync(resolve(root, "mise.toml"), "utf8"), /-f ai-gateway\/Dockerfile/);
    assert.match(readFileSync(resolve(root, "ai-gateway/Dockerfile.dockerignore"), "utf8"), /go\.gateway\.work/);
  });
});
