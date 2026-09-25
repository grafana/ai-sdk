import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, it } from "node:test";
import { parseDocument } from "yaml";

const root = resolve(import.meta.dirname, "../../..");
const document = parseDocument(readFileSync(resolve(root, ".github/workflows/ci.yml"), "utf8"));
assert.deepEqual(document.errors, []);
const workflow = document.toJS() as { jobs: Record<string, { if?: string; needs?: string | string[]; steps: Array<{ name?: string; run?: string; uses?: string; "continue-on-error"?: boolean }> }> };
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

  it("validates standalone Gateway before images and fails closed before publication", () => {
    assert.equal(normalize(jobs["image-validation"]!.if), pushGuard);
    const steps = jobs["image-validation"]!.steps;
    const standalone = steps.findIndex((step) => step.name === "Verify standalone Gateway at the artifact revision");
    assert.ok(standalone > 0);
    for (const name of ["Build target platforms", "Build native image", "Smoke test native image with IPv6 disabled"]) {
      assert.ok(steps.findIndex((step) => step.name === name) > standalone, `${name} must follow standalone validation`);
    }
    assert.match(steps[standalone]!.run!, /test "\$\(git rev-parse HEAD\)" = "\$GITHUB_SHA"/);
    assert.match(steps[standalone]!.run!, /MODULE=ai-gateway mise run verify-published-module/);
    assert.equal(steps[standalone]!["continue-on-error"], undefined);
    assert.ok(steps.slice(0, standalone).some((step) => step.uses?.startsWith("actions/checkout@")));
    assert.ok(steps.slice(0, standalone).some((step) => step.uses?.startsWith("jdx/mise-action@")));
    assert.match(commands("image-validation"), /VCS_REF="\$GITHUB_SHA"/);
    assert.match(commands("image-validation"), /build-ai-gateway-image/);
    assert.equal(normalize(jobs["publish-ai-gateway-image"]!.if), pushGuard);
    for (const required of [...source, "image-validation"]) {
      assert.ok(dependencies("publish-ai-gateway-image").has(required), `publisher must require ${required}`);
    }
    assert.match(commands("publish-ai-gateway-image"), /VCS_REF=\$GITHUB_SHA/);
    assert.equal(normalize(jobs["deploy-ai-gateway"]!.if), "${{ github.event_name == 'push' && github.ref == 'refs/heads/main' && github.repository == 'grafana/ai-sdk' }}");
    assert.ok(dependencies("deploy-ai-gateway").has("publish-ai-gateway-image"));
    const dockerfile = readFileSync(resolve(root, "ai-gateway/Dockerfile"), "utf8");
    assert.match(dockerfile, /GOWORK=off/);
    assert.doesNotMatch(dockerfile, /go\.gateway\.work|COPY.*go\.work/);
  });
});
