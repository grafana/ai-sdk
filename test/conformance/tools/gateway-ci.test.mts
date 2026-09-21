import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import { parse } from "yaml";

const repository = resolve(dirname(fileURLToPath(import.meta.url)), "../../..");

test("gateway CI remains an independent failing check, not a publish prerequisite", () => {
  const workflow = parse(readFileSync(resolve(repository, ".github/workflows/ci.yml"), "utf8"));
  const job = workflow.jobs["gateway-conformance-test"];
  assert.equal(job.name, "Gateway conformance (advisory)");
  assert.equal(job["continue-on-error"], undefined);
  assert.equal(job.needs, undefined);
  const mise = job.steps.find((step: { uses?: string }) => step.uses?.startsWith("jdx/mise-action@"));
  assert.equal(mise?.with?.cache, false, "gateway CI must not restore or save runtime caches");
  assert.ok(job.steps.some((step: { run?: string }) => step.run === "mise run test-conformance-gateway"));
  assert.ok(job.steps.some((step: { uses?: string; if?: string }) => step.uses?.startsWith("actions/upload-artifact@") && step.if === "always()"));
  for (const step of job.steps) assert.equal(step["continue-on-error"], undefined);
  for (const [name, other] of Object.entries(workflow.jobs) as [string, { needs?: string | string[] }][]) {
    if (name === "gateway-conformance-test") continue;
    const needs = typeof other.needs === "string" ? [other.needs] : other.needs ?? [];
    assert.ok(!needs.includes("gateway-conformance-test"), `${name} must not depend on advisory conformance`);
  }
  assert.ok(workflow.jobs["publish-ai-gateway-image"].needs.includes("conformance-test"));
});
