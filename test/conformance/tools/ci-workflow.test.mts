import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { relative, resolve } from "node:path";
import { describe, it } from "node:test";
import { parseAllDocuments, parseDocument } from "yaml";

const root = resolve(import.meta.dirname, "../../..");
const document = parseDocument(readFileSync(resolve(root, ".github/workflows/ci.yml"), "utf8"));
assert.deepEqual(document.errors, []);
const workflow = document.toJS() as { concurrency: { group: string; "cancel-in-progress": string }; on: { push: { branches: string[]; tags?: string[] }; pull_request: unknown }; jobs: Record<string, { if?: string; needs?: string | string[]; permissions?: Record<string, string>; "continue-on-error"?: boolean; steps: Array<{ name?: string; if?: string; run?: string; uses?: string; env?: Record<string, string>; with?: Record<string, unknown>; "continue-on-error"?: boolean }> }> };
const jobs = workflow.jobs;
const source = ["ci", "race", "docs-lint", "module-resolution", "parity-baseline", "integration-test", "direct-conformance-test"];
const pushGuard = "${{ github.event_name == 'push' && (github.ref == 'refs/heads/main' || startsWith(github.ref, 'refs/tags/ai-gateway/v')) && github.repository == 'grafana/ai-sdk' }}";
const normalize = (value: string | undefined) => value?.replace(/\s+/g, " ").trim();
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
      assert.ok(!required.has("plugin-validation"), `${job} depends on the artifact gate`);
      for (const path of [job, ...required]) {
        assert.doesNotMatch(commands(path), /verify-module-resolution|verify-published-module|test-candidate-source|test-go-client-standalone/);
      }
    }
    for (const task of ["test-module-policy", "test-ci-workflow", "verify-gateway-workspace", "verify-ai-gateway-boundary", "verify-merged-pins", "verify-sdk-gateway-isolation"]) {
      assert.match(commands("module-resolution"), new RegExp(`mise run ${task}`));
    }
    assert.match(commands("parity-baseline"), /test-providerwire-v4/);
    assert.match(commands("direct-conformance-test"), /test-conformance/);
    assert.match(commands("integration-test"), /test-integration/);
    for (const task of ["fmt-check", "release-check", "vet", "build", "test-short"]) {
      assert.match(commands("build-test"), new RegExp(`mise run ${task}\\b`));
    }
    assert.match(commands("lint"), /mise run lint/);
    assert.match(commands("race"), /mise run test-race/);
  });

  it("validates the plugin package on pull requests and keeps it only for eligible pushes", () => {
    const job = jobs["plugin-validation"];
    assert.ok(job, "missing plugin validation job");
    assert.equal(job.if, undefined, "plugin validation must run on PR and push");
    const steps = job.steps;
    assert.ok(steps.some((step) => step.uses?.startsWith("actions/checkout@") && step.with?.["persist-credentials"] === false));
    assert.ok(steps.every((step) => step["continue-on-error"] !== true));
    const clean = steps.find((step) => step.name === "Verify clean plugin source at the artifact revision")!;
    assert.equal(normalize(clean.if), "${{ github.event_name == 'push' }}");
    assert.match(clean.run!, /git rev-parse HEAD.*GITHUB_SHA/);
    assert.match(clean.run!, /git status --porcelain --untracked-files=all/);
    const order = ["generate:kinds", "mise run test-ai-gateway-frontend", "mise run build-ai-gateway-plugin", "mise run validate-ai-gateway-plugin", "mise run test-ai-gateway-e2e", "mise run test-ai-gateway\n", "mise run test-ai-gateway-command"];
    const script = `${commands("plugin-validation")}\n`;
    let position = -1;
    for (const command of order) {
      const next = script.indexOf(command, position + 1);
      assert.ok(next > position, `${command} must run in order`);
      position = next;
    }
    assert.match(commands("plugin-validation"), /git status --porcelain --untracked-files=all -- ai-gateway\)"/);
    const e2e = steps.find((step) => step.run === "mise run test-ai-gateway-e2e")!;
    assert.match(e2e.env?.GRAFANA_VERSION ?? "", /^nightly@sha256:[0-9a-f]{64}$/);
    const build = steps.find((step) => step.run === "mise run build-ai-gateway-plugin")!;
    assert.equal(build.env?.AI_GATEWAY_VCS_REF, "${{ github.sha }}");
    const caches = steps.filter((step) => step.uses?.startsWith("actions/cache@"));
    assert.equal(caches.length, 2, "plugin validation caches Go and pnpm");
    for (const cache of caches) assert.equal(normalize(cache.with?.["lookup-only"] as string), "${{ startsWith(github.ref, 'refs/tags/') }}");
    const keep = steps.at(-1)!;
    assert.match(keep.uses!, /^actions\/upload-artifact@[0-9a-f]{40}$/);
    assert.equal(normalize(keep.if), pushGuard);
    assert.equal(keep.with?.path, "ai-gateway/dist");
    assert.equal(keep.with?.["if-no-files-found"], "error");
    assert.doesNotMatch(commands("plugin-validation"), /GOWORK=off|verify-published-module/);
  });

  it("has no image, publication or deployment jobs", () => {
    for (const job of ["image-validation", "publish-ai-gateway-image", "deploy-ai-gateway"]) assert.equal(jobs[job], undefined, job);
    for (const job of Object.keys(jobs)) {
      assert.doesNotMatch(commands(job), /docker|Dockerfile|ghcr\.io/i, job);
      for (const step of jobs[job]!.steps) assert.doesNotMatch(step.uses ?? "", /docker\/|trigger-argo-workflow/, job);
    }
    assert.doesNotMatch(readFileSync(resolve(root, "mise.toml"), "utf8"), /Dockerfile|build-ai-gateway-image|test-ai-gateway-image-source/);
  });

  it("keeps the required source checks and workers without image automation", () => {
    assert.deepEqual(Object.keys(jobs).sort(), [...source, "build-test", "lint", "plugin-validation", "gateway-conformance-test", "conformance-test"].sort());
    assert.deepEqual(workflow.on.push, { branches: ["main"], tags: ["ai-gateway/v*"] });
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
    for (const job of ["build-test", "lint", "race", "module-resolution", "parity-baseline", "integration-test", "direct-conformance-test", "gateway-conformance-test"]) {
      const steps = jobs[job]!.steps;
      const cacheIndex = steps.findIndex((step) => step.uses === "./.github/actions/cache-go");
      assert.ok(cacheIndex > steps.findIndex((step) => step.uses?.startsWith("actions/checkout@")), job);
      assert.ok(cacheIndex < steps.findIndex((step) => step.run), job);
    }
  });
});
