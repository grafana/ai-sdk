import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import { parse } from "yaml";

const repository = resolve(dirname(fileURLToPath(import.meta.url)), "../../..");

test("gateway CI blocks the required conformance check without gating publication", () => {
  const workflow = parse(readFileSync(resolve(repository, ".github/workflows/ci.yml"), "utf8"));
  const job = workflow.jobs["gateway-conformance-test"];
  assert.equal(job.name, "Gateway conformance");
  assert.equal(job["continue-on-error"], undefined);
  assert.equal(job.needs, undefined);
  const mise = job.steps.find((step: { uses?: string }) => step.uses?.startsWith("jdx/mise-action@"));
  assert.equal(mise?.with?.cache, false, "gateway CI must not restore or save runtime caches");
  assert.ok(job.steps.some((step: { run?: string }) => step.run === "mise run test-conformance-gateway"));
  assert.ok(job.steps.some((step: { uses?: string; if?: string }) => step.uses?.startsWith("actions/upload-artifact@") && step.if === "always()"));
  for (const step of job.steps) assert.equal(step["continue-on-error"], undefined);
  const required = workflow.jobs["conformance-test"];
  assert.equal(required.name, "conformance-test");
  assert.equal(required.if, "${{ always() }}");
  assert.deepEqual(required.needs, ["direct-conformance-test", "gateway-conformance-test"]);
  assert.equal(required["continue-on-error"], undefined);
  const gate = required.steps.find((step: { name?: string }) => step.name === "Require both conformance suites");
  assert.match(gate?.run ?? "", /test "\$DIRECT_RESULT" = success && test "\$GATEWAY_RESULT" = success/);
  assert.equal(gate?.env?.DIRECT_RESULT, "${{ needs.direct-conformance-test.result }}");
  assert.equal(gate?.env?.GATEWAY_RESULT, "${{ needs.gateway-conformance-test.result }}");
  const publisher = workflow.jobs["publish-ai-gateway-image"];
  assert.ok(publisher.needs.includes("direct-conformance-test"));
  assert.ok(!publisher.needs.includes("gateway-conformance-test"));
  assert.ok(!publisher.needs.includes("conformance-test"));
  for (const [name, other] of Object.entries(workflow.jobs) as [string, { needs?: string | string[] }][]) {
    if (name === "conformance-test") continue;
    const needs = typeof other.needs === "string" ? [other.needs] : other.needs ?? [];
    assert.ok(!needs.includes("gateway-conformance-test"), `${name} must not depend directly on gateway conformance`);
  }
});
