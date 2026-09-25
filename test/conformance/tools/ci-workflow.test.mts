import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, it } from "node:test";
import { parseDocument } from "yaml";

const root = resolve(import.meta.dirname, "../../..");
const document = parseDocument(readFileSync(resolve(root, ".github/workflows/ci.yml"), "utf8"));
assert.deepEqual(document.errors, []);
const workflow = document.toJS() as { jobs: Record<string, { if?: string; needs?: string | string[]; steps: Array<{ name?: string; run?: string; id?: string; "continue-on-error"?: boolean }> }> };
const jobs = workflow.jobs;
const source = ["ci", "docs-lint", "module-resolution", "parity-baseline", "integration-test", "conformance-test"];
const artifact = ["artifact-standalone", "image-validation"];
const allowedPush = "${{ github.event_name == 'push' && (github.ref == 'refs/heads/main' || startsWith(github.ref, 'refs/tags/ai-gateway/v')) && github.repository == 'grafana/ai-sdk' }}";
const normalize = (value: string | undefined) => value?.replace(/\s+/g, " ").trim();
const needs = (job: string) => Array.isArray(jobs[job]!.needs) ? jobs[job]!.needs as string[] : jobs[job]!.needs ? [jobs[job]!.needs as string] : [];
const commands = (job: string) => jobs[job]!.steps.map((step) => step.run ?? "").join("\n");

function eligible(event: "pull_request" | "push", ref: string, repository: string): boolean {
  return event === "push" && repository === "grafana/ai-sdk" && (ref === "refs/heads/main" || ref.startsWith("refs/tags/ai-gateway/v"));
}

function succeeds(results: Record<string, string>, job: string): boolean {
  return needs(job).every((dependency) => results[dependency] === "success");
}

describe("source and artifact workflow gates", () => {
  it("keeps source checks independent of standalone and image diagnostics", () => {
    for (const job of source) {
      assert.ok(jobs[job], `missing source check ${job}`);
      assert.equal(jobs[job]!.if, undefined, `${job} must run on PR, main and Gateway tags`);
      assert.deepEqual(needs(job), []);
    }
    assert.doesNotMatch(commands("module-resolution"), /verify-module-resolution/);
    assert.match(commands("module-resolution"), /verify-merged-pins/);
    assert.match(commands("module-resolution"), /verify-ai-gateway-boundary/);
    assert.match(commands("module-resolution"), /verify-sdk-gateway-isolation/);
    assert.match(commands("module-resolution"), /test-candidate-source/);
    assert.match(commands("parity-baseline"), /test-providerwire-v4/);
    assert.match(commands("conformance-test"), /test-conformance/);
    assert.match(commands("integration-test"), /test-integration/);
    assert.match(commands("ci"), /mise run (build|test-short|vet|lint)/);
    assert.equal(jobs["standalone-diagnostic"]!.if, "github.event_name == 'pull_request'");
    assert.match(commands("standalone-diagnostic"), /verify-module-resolution/);
    assert.match(commands("standalone-diagnostic"), /test-go-client-standalone/);
    for (const id of ["standalone", "client"]) {
      assert.equal(jobs["standalone-diagnostic"]!.steps.find((step) => step.id === id)?.["continue-on-error"], true);
    }
    assert.match(commands("standalone-diagnostic"), /GITHUB_STEP_SUMMARY/);
    assert.ok(!needs("publish-ai-gateway-image").includes("standalone-diagnostic"));
  });

  it("requires exact-revision standalone and image evidence before publication", () => {
    for (const job of artifact) assert.equal(normalize(jobs[job]!.if), allowedPush);
    assert.deepEqual(needs("image-validation"), ["artifact-standalone"]);
    assert.match(commands("artifact-standalone"), /MODULE=ai-gateway mise run verify-published-module/);
    assert.match(commands("image-validation"), /VCS_REF="\$GITHUB_SHA"/);
    assert.match(commands("image-validation"), /build-ai-gateway-image/);
    assert.equal(normalize(jobs["publish-ai-gateway-image"]!.if), allowedPush);
    assert.deepEqual(new Set(needs("publish-ai-gateway-image")), new Set([...source, ...artifact]));
    assert.match(commands("publish-ai-gateway-image"), /VCS_REF=\$GITHUB_SHA/);
    assert.equal(normalize(jobs["deploy-ai-gateway"]!.if), "${{ github.event_name == 'push' && github.ref == 'refs/heads/main' && github.repository == 'grafana/ai-sdk' }}");
    assert.deepEqual(needs("deploy-ai-gateway"), ["publish-ai-gateway-image"]);
    assert.match(readFileSync(resolve(root, "ai-gateway/Dockerfile"), "utf8"), /GOWORK=off/);
    assert.doesNotMatch(readFileSync(resolve(root, "ai-gateway/Dockerfile"), "utf8"), /go\.gateway\.work|COPY.*go\.work/);
  });

  it("keeps failure, cancellation and skip fail-closed for each event", () => {
    for (const event of [
      ["pull_request", "refs/pull/1/merge", "grafana/ai-sdk", false, false],
      ["push", "refs/heads/topic", "grafana/ai-sdk", false, false],
      ["push", "refs/heads/main", "fork/ai-sdk", false, false],
      ["push", "refs/tags/other/v1", "grafana/ai-sdk", false, false],
      ["push", "refs/heads/main", "grafana/ai-sdk", true, true],
      ["push", "refs/tags/ai-gateway/v1", "grafana/ai-sdk", true, false],
    ] as const) {
      const [kind, ref, repo, publish, deploy] = event;
      assert.equal(eligible(kind, ref, repo), publish);
      const results = Object.fromEntries([...source, ...artifact].map((job) => [job, "success"]));
      assert.equal(eligible(kind, ref, repo) && succeeds(results, "publish-ai-gateway-image"), publish);
      assert.equal(ref === "refs/heads/main" && publish, deploy);
      results["standalone-diagnostic"] = "failure";
      assert.equal(eligible(kind, ref, repo) && succeeds(results, "publish-ai-gateway-image"), publish);
      for (const dependency of [...source, ...artifact]) for (const state of ["failure", "cancelled", "skipped"]) {
        results[dependency] = state;
        assert.equal(succeeds(results, "publish-ai-gateway-image"), false, `${dependency}: ${state}`);
        results[dependency] = "success";
      }
      results["artifact-standalone"] = "failure";
      assert.equal(succeeds(results, "image-validation"), false);
      assert.equal(succeeds({ "publish-ai-gateway-image": "skipped" }, "deploy-ai-gateway"), false);
    }
  });
});
