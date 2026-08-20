import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  generateClassificationArtifact,
  valueAtPointer,
} from "./classification.ts";
import {
  baselineMetadata,
  validateBaselineMetadata,
} from "./baseline.ts";

function staleMetadata() {
  const metadata = structuredClone(baselineMetadata());
  metadata.commit = "stale";
  metadata.packages["@ai-sdk/gateway"] = "stale";
  return metadata;
}

describe("request-surface classification", () => {
  it("derives baseline metadata from the registered baseline", () => {
    const classification = generateClassificationArtifact();
    assert.deepEqual(classification.baseline, baselineMetadata());
    assert.deepEqual(
      validateBaselineMetadata("classification artifact", classification.baseline),
      [],
    );
  });

  it("rejects stale baseline metadata", () => {
    const errors = validateBaselineMetadata("classification artifact", staleMetadata());
    assert.ok(errors.some((error) => error.includes("commit stale")));
    assert.ok(errors.some((error) => error.includes("@ai-sdk/gateway version stale")));
  });

  it("resolves semantic JSON pointers without truthiness loss", () => {
    assert.deepEqual(valueAtPointer({ values: [false, 0, ""] }, "/values/0"), {
      found: true,
      value: false,
    });
    assert.deepEqual(valueAtPointer({ values: [false] }, "/values/1"), { found: false });
  });
});
