import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  sourceInputs,
  validateSourceEquivalenceEvidence,
  type SourceEquivalenceEvidence,
} from "./source-equivalence.ts";

const baseline = {
  upstream: { repository: "https://example.invalid/ai", commit: "a".repeat(40) },
  packages: {
    ai: "7.0.65",
    "@ai-sdk/gateway": "4.0.52",
    "@ai-sdk/provider": "4.0.7",
    "@ai-sdk/provider-utils": "5.0.27",
  },
};
const packageManifest = { dependencies: { ...baseline.packages } };

function evidence(hash: string): SourceEquivalenceEvidence {
  return {
    schemaVersion: 1,
    upstream: { ...baseline.upstream },
    packages: { ...baseline.packages },
    files: sourceInputs.map((input) => ({ ...input, sha256: hash })),
  };
}

describe("source-equivalence evidence", () => {
  it("covers the explicit request, transport, and error source closure", () => {
    const installedPaths = new Set(
      sourceInputs.map((input) => `${input.package}/${input.installedPath}`),
    );
    for (const path of [
      "@ai-sdk/gateway/src/gateway-language-model.ts",
      "@ai-sdk/gateway/src/gateway-provider.ts",
      "@ai-sdk/gateway/src/errors/as-gateway-error.ts",
      "@ai-sdk/provider/src/language-model/v4/language-model-v4-call-options.ts",
      "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
      "@ai-sdk/provider-utils/src/cancel-response-body.ts",
      "@ai-sdk/provider-utils/src/download-error.ts",
      "@ai-sdk/provider-utils/src/post-to-api.ts",
      "@ai-sdk/provider-utils/src/normalize-headers.ts",
      "@ai-sdk/provider-utils/src/response-handler.ts",
      "@ai-sdk/provider-utils/src/load-optional-setting.ts",
      "@ai-sdk/provider-utils/src/without-trailing-slash.ts",
    ]) {
      assert.equal(installedPaths.has(path), true, `missing transitive source ${path}`);
    }
    assert.equal(installedPaths.size, sourceInputs.length);
    assert.ok(sourceInputs.length < 100);
    assert.equal(installedPaths.has("@ai-sdk/gateway/src/gateway-image-model.ts"), false);
  });

  it("rejects a missing covered source", () => {
    const fixture = evidence("hash");
    fixture.files.pop();
    const errors = validateSourceEquivalenceEvidence(
      fixture,
      baseline,
      packageManifest,
      () => Buffer.from("installed"),
    );
    assert.ok(errors.some((error) => error.startsWith("missing source-equivalence entry")));
  });

  it("rejects installed source drift", () => {
    const errors = validateSourceEquivalenceEvidence(
      evidence("stale"),
      baseline,
      packageManifest,
      () => Buffer.from("installed"),
    );
    assert.ok(errors.some((error) => error.includes("installed hash") && error.includes("stale")));
  });

  it("rejects a missing required baseline package pin", () => {
    const dependencies: Record<string, string> = { ...baseline.packages };
    delete dependencies.ai;
    const errors = validateSourceEquivalenceEvidence(
      evidence("unused"),
      baseline,
      { dependencies },
      () => Buffer.from("installed"),
    );
    assert.ok(errors.some((error) => error.includes("ai workspace version")));
  });

  it("rejects unsupported metadata shape", () => {
    const fixture = evidence("unused");
    (fixture as { schemaVersion: number }).schemaVersion = 99;
    fixture.packages["@ai-sdk/react"] = "stale";
    const errors = validateSourceEquivalenceEvidence(
      fixture,
      baseline,
      packageManifest,
      () => Buffer.from("installed"),
    );
    assert.ok(errors.some((error) => error.includes("schema version 99")));
    assert.ok(errors.some((error) => error.includes("package keys")));
  });

  it("rejects stale source-equivalence baseline metadata", () => {
    const fixture = evidence("unused");
    fixture.upstream.commit = "stale";
    fixture.packages["@ai-sdk/provider-utils"] = "stale";
    const errors = validateSourceEquivalenceEvidence(
      fixture,
      baseline,
      packageManifest,
      () => Buffer.from("installed"),
    );
    assert.ok(errors.some((error) => error.includes("commit stale")));
    assert.ok(errors.some((error) => error.includes("@ai-sdk/provider-utils version stale")));
  });

  it("rejects exact package-version drift", () => {
    const errors = validateSourceEquivalenceEvidence(
      evidence("unused"),
      baseline,
      {
        dependencies: { ...baseline.packages, "@ai-sdk/gateway": "4.0.53" },
      },
      () => Buffer.from("installed"),
    );
    assert.ok(errors.some((error) => error.includes("@ai-sdk/gateway workspace version")));
  });
});
