import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { relative, resolve } from "node:path";
import { describe, it } from "node:test";
import { parseAllDocuments, parseDocument } from "yaml";

const root = resolve(import.meta.dirname, "../../..");
const document = parseDocument(readFileSync(resolve(root, ".github/workflows/ci.yml"), "utf8"));
assert.deepEqual(document.errors, []);
const workflow = document.toJS() as { on: { push: { branches: string[]; tags?: string[] }; pull_request: unknown }; jobs: Record<string, { if?: string; needs?: string | string[]; permissions?: Record<string, string>; "continue-on-error"?: boolean; steps: Array<{ name?: string; run?: string; uses?: string; with?: { "persist-credentials"?: boolean }; "continue-on-error"?: boolean }> }> };
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
      assert.equal(jobs[job]!.if, undefined, `${job} must run on PR and push`);
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
    assert.match(commands("ci"), /mise run (build|test-short|vet|lint)/);
    assert.match(commands("race"), /mise run test-race/);
  });

  it("runs only the seven source checks on PRs and main pushes without image automation", () => {
    assert.deepEqual(Object.keys(jobs).sort(), [...source].sort());
    assert.deepEqual(workflow.on.push, { branches: ["main"] });
    assert.ok(Object.hasOwn(workflow.on, "pull_request"));
    assert.deepEqual(Object.keys(workflow.on).sort(), ["pull_request", "push"]);
    for (const job of source) {
      dependencies(job);
      assert.notEqual(jobs[job]!.permissions?.packages, "write");
      assert.doesNotMatch(commands(job), /docker (build|buildx|run|login|push)|build-ai-gateway-image|test-ai-gateway-image-source/);
      assert.ok(jobs[job]!.steps.every((step) => !step.uses?.startsWith("docker/")));
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
