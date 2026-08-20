import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, it } from "node:test";
import {
  compareSemanticRequestsArtifact,
  semanticRequestsPath,
  serializeArtifact,
  type SemanticRequestsArtifact,
} from "./artifacts.ts";
import { baselineMetadata } from "./baseline.ts";
import { classificationPath } from "./classification.ts";
import {
  sourceEquivalencePath,
  type SourceEquivalenceEvidence,
} from "./source-equivalence.ts";
import { checkProviderWireV4 } from "./check.ts";

const artifact: SemanticRequestsArtifact = {
  schemaVersion: 1,
  baseline: baselineMetadata(),
  capturePolicy: {
    path: "relative",
    normalizedHeaders: [],
    excludedTransportHeaders: [],
    preserved: [],
  },
  scenarios: [],
};

describe("non-mutating artifact comparison", () => {
  it("accepts matching evidence without writing", () => {
    const directory = mkdtempSync(join(tmpdir(), "providerwire-v4-artifact-"));
    const path = join(directory, "artifact.json");
    try {
      const contents = serializeArtifact(artifact);
      writeFileSync(path, contents);
      assert.deepEqual(compareSemanticRequestsArtifact(artifact, path), []);
      assert.equal(readFileSync(path, "utf8"), contents);
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });

  it("reports stale evidence without rewriting it", () => {
    const directory = mkdtempSync(join(tmpdir(), "providerwire-v4-artifact-"));
    const path = join(directory, "artifact.json");
    try {
      const stale = "{\"stale\":true}\n";
      writeFileSync(path, stale);
      const errors = compareSemanticRequestsArtifact(artifact, path);
      assert.ok(errors.some((error) => error.includes("stale")));
      assert.equal(readFileSync(path, "utf8"), stale);
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });

  it("rejects stale semantic baseline metadata without rewriting it", async () => {
    const directory = mkdtempSync(join(tmpdir(), "providerwire-v4-semantic-baseline-"));
    const path = join(directory, "semantic-requests.json");
    try {
      const stale = JSON.parse(readFileSync(semanticRequestsPath, "utf8")) as SemanticRequestsArtifact;
      stale.baseline.commit = "stale";
      const contents = serializeArtifact(stale);
      writeFileSync(path, contents);
      await assert.rejects(checkProviderWireV4(path), /semantic request evidence is stale/);
      assert.equal(readFileSync(path, "utf8"), contents);
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });

  it("rejects stale classification baseline metadata without rewriting it", async () => {
    const directory = mkdtempSync(join(tmpdir(), "providerwire-v4-classification-baseline-"));
    const path = join(directory, "classification.json");
    try {
      const stale = JSON.parse(readFileSync(classificationPath, "utf8")) as {
        baseline: { commit: string };
      };
      stale.baseline.commit = "stale";
      const contents = serializeArtifact(stale);
      writeFileSync(path, contents);
      await assert.rejects(
        checkProviderWireV4(undefined, path),
        /classification evidence is stale/,
      );
      assert.equal(readFileSync(path, "utf8"), contents);
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });

  it("rejects stale source-equivalence metadata without rewriting it", async () => {
    const directory = mkdtempSync(join(tmpdir(), "providerwire-v4-source-baseline-"));
    const path = join(directory, "source-equivalence.json");
    try {
      const stale = JSON.parse(
        readFileSync(sourceEquivalencePath, "utf8"),
      ) as SourceEquivalenceEvidence;
      stale.upstream.commit = "stale";
      const contents = serializeArtifact(stale);
      writeFileSync(path, contents);
      await assert.rejects(
        checkProviderWireV4(undefined, undefined, path),
        /source equivalence commit stale/,
      );
      assert.equal(readFileSync(path, "utf8"), contents);
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });

  it("runs the complete evidence check against stale storage without rewriting it", async () => {
    const directory = mkdtempSync(join(tmpdir(), "providerwire-v4-check-"));
    const path = join(directory, "semantic-requests.json");
    try {
      const stale = "{\"stale\":true}\n";
      writeFileSync(path, stale);
      await assert.rejects(checkProviderWireV4(path), /semantic request evidence is stale/);
      assert.equal(readFileSync(path, "utf8"), stale);
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });
});
