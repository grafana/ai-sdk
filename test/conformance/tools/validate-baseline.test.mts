import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { defaultPackagePaths, validateBaseline } from "./validate-baseline.mts";

describe("validateBaseline", () => {
  it("registers every parity package consumer", () => {
    for (const suffix of [
      "/test/conformance/tools/package.json",
      "/test/integration/package.json",
      "/test/interop/package.json",
      "/test/cli/package.json",
      "/test/providerwire-v4/package.json",
    ]) {
      assert.ok(
        defaultPackagePaths.some((packagePath) => packagePath.endsWith(suffix)),
        suffix,
      );
    }
  });

  it("accepts matching ProviderWire V4 package versions", () => {
    const errors = validateBaseline(
      {
        packages: {
          ai: "7.0.65",
          "@ai-sdk/gateway": "4.0.52",
          "@ai-sdk/provider": "4.0.7",
        },
      },
      {
        dependencies: {
          ai: "7.0.65",
          "@ai-sdk/gateway": "4.0.52",
          "@ai-sdk/provider": "4.0.7",
        },
      },
      "test/providerwire-v4/package.json",
    );

    assert.deepEqual(errors, []);
  });

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
});
