import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  defaultPackagePaths,
  providerWireRequiredPackages,
  validateBaseline,
} from "./validate-baseline.mts";

describe("validateBaseline", () => {
  it("accepts matching AI SDK package versions", () => {
    const errors = validateBaseline(
      {
        packages: {
          ai: "7.0.0-beta.116",
          "@ai-sdk/anthropic": "4.0.0-beta.42",
        },
      },
      {
        dependencies: {
          ai: "7.0.0-beta.116",
          "@ai-sdk/anthropic": "4.0.0-beta.42",
          yaml: "^2.7.0",
        },
      },
    );

    assert.deepEqual(errors, []);
  });

  it("reports mismatched package versions", () => {
    const errors = validateBaseline(
      {
        packages: {
          ai: "7.0.0-beta.117",
        },
      },
      {
        dependencies: {
          ai: "7.0.0-beta.116",
        },
      },
    );

    assert.deepEqual(errors, [
      "package.json dependency ai pins 7.0.0-beta.116, but baseline declares 7.0.0-beta.117",
    ]);
  });

  it("reports AI SDK dependencies missing from the baseline", () => {
    const errors = validateBaseline(
      {
        packages: {
          ai: "7.0.0-beta.116",
        },
      },
      {
        dependencies: {
          ai: "7.0.0-beta.116",
          "@ai-sdk/anthropic": "4.0.0-beta.42",
        },
      },
    );

    assert.deepEqual(errors, [
      "package.json dependency @ai-sdk/anthropic@4.0.0-beta.42 is missing from baseline manifest",
    ]);
  });

  it("checks devDependencies", () => {
    const errors = validateBaseline(
      {
        packages: {
          ai: "7.0.0-beta.116",
          "@ai-sdk/react": "4.0.0-beta.116",
        },
      },
      {
        devDependencies: {
          ai: "7.0.0-beta.116",
          "@ai-sdk/react": "4.0.0-beta.115",
        },
      },
      "test/integration/package.json",
    );

    assert.deepEqual(errors, [
      "test/integration/package.json dependency @ai-sdk/react pins 4.0.0-beta.115, but baseline declares 4.0.0-beta.116",
    ]);
  });

  it("includes the ProviderWire V4 contract workspace by default", () => {
    const paths = defaultPackagePaths("/repo/test/conformance/tools");

    assert.equal(
      paths.some((path) => path.endsWith("/ai-gateway/test/providerwire-v4/package.json")),
      true,
    );
  });

  it("labels ProviderWire dependency drift", () => {
    const errors = validateBaseline(
      { packages: { "@ai-sdk/gateway": "4.0.52" } },
      { dependencies: { "@ai-sdk/gateway": "4.0.51" } },
      "ai-gateway/test/providerwire-v4/package.json",
    );

    assert.deepEqual(errors, [
      "ai-gateway/test/providerwire-v4/package.json dependency @ai-sdk/gateway pins 4.0.51, but baseline declares 4.0.52",
    ]);
  });

  it("rejects omitted required ProviderWire dependencies", () => {
    const baseline = {
      packages: {
        "@ai-sdk/gateway": "4.0.52",
        "@ai-sdk/provider": "4.0.7",
        "@ai-sdk/provider-utils": "5.0.27",
      },
    };

    for (const omitted of providerWireRequiredPackages) {
      const dependencies = Object.fromEntries(
        providerWireRequiredPackages
          .filter((name) => name !== omitted)
          .map((name) => [name, baseline.packages[name]]),
      );
      const errors = validateBaseline(
        baseline,
        { dependencies },
        "ai-gateway/test/providerwire-v4/package.json",
        providerWireRequiredPackages,
      );

      assert.deepEqual(errors, [
        `ai-gateway/test/providerwire-v4/package.json must declare dependency ${omitted}@${baseline.packages[omitted]}`,
      ]);
    }
  });
});
