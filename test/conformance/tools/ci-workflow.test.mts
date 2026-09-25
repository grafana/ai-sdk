import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, it } from "node:test";
import { parseDocument } from "yaml";

const root = resolve(import.meta.dirname, "../../..");
const document = parseDocument(readFileSync(resolve(root, ".github/workflows/ci.yml"), "utf8"));
assert.deepEqual(document.errors, []);
const workflow = document.toJS() as { jobs: Record<string, { if?: string; needs?: string | string[]; steps: Array<{ name?: string; run?: string; uses?: string; with?: { "persist-credentials"?: boolean }; "continue-on-error"?: boolean }> }> };
const jobs = workflow.jobs;
const source = ["ci", "docs-lint", "module-resolution", "parity-baseline", "integration-test", "conformance-test"];
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

describe("source and artifact workflow gates", () => {
  it("keeps candidate-source checks blocking without standalone prerequisites", () => {
    for (const job of source) {
      assert.ok(jobs[job], `missing source check ${job}`);
      assert.equal(jobs[job]!.if, undefined, `${job} must run on PR and push`);
      const required = dependencies(job);
      assert.ok(!required.has("image-validation"), `${job} depends on the artifact gate`);
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
  });

  it("builds workspace-source images at the push SHA and gates publication on source and image checks", () => {
    assert.equal(normalize(jobs["image-validation"]!.if), pushGuard);
    assert.equal(normalize(jobs["publish-ai-gateway-image"]!.if), pushGuard);
    for (const job of ["image-validation", "publish-ai-gateway-image"]) {
      const steps = jobs[job]!.steps;
      assert.ok(steps.some((step) => step.uses?.startsWith("actions/checkout@") && step.with?.["persist-credentials"] === false));
      assert.match(commands(job), /git rev-parse HEAD/);
      assert.match(commands(job), /GITHUB_SHA/);
      assert.match(commands(job), /git status --porcelain/);
      assert.doesNotMatch(commands(job), /verify-published-module|GOWORK=off/);
      assert.ok(steps.every((step) => step["continue-on-error"] !== true));
    }
    assert.match(commands("image-validation"), /--platform linux\/amd64,linux\/arm64/);
    assert.match(commands("image-validation"), /-f ai-gateway\/Dockerfile/);
    assert.match(commands("image-validation"), /build-ai-gateway-image/);
    assert.ok(jobs["image-validation"]!.steps.some((step) => step.name === "Smoke test native image with IPv6 disabled"));
    assert.match(commands("publish-ai-gateway-image"), /-f ai-gateway\/Dockerfile/);
    assert.match(commands("publish-ai-gateway-image"), /VCS_REF=\$GITHUB_SHA/);
    for (const required of [...source, "image-validation"]) {
      assert.ok(dependencies("publish-ai-gateway-image").has(required), `publisher must require ${required}`);
    }
    assert.equal(normalize(jobs["deploy-ai-gateway"]!.if), "${{ github.event_name == 'push' && github.ref == 'refs/heads/main' && github.repository == 'grafana/ai-sdk' }}");
    assert.deepEqual(needs("deploy-ai-gateway"), ["publish-ai-gateway-image"]);
    const dockerfile = readFileSync(resolve(root, "ai-gateway/Dockerfile"), "utf8");
    assert.match(dockerfile, /GOWORK=\/src\/go\.gateway\.work/);
    assert.doesNotMatch(dockerfile, /GOWORK=off/);
    assert.match(readFileSync(resolve(root, "mise.toml"), "utf8"), /-f ai-gateway\/Dockerfile/);
    assert.match(readFileSync(resolve(root, "ai-gateway/Dockerfile.dockerignore"), "utf8"), /go\.gateway\.work/);
  });
});
