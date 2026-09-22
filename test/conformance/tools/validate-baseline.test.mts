import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, it } from "node:test";
import { collectGatewayContract, readGatewayContractEvidence, validateGatewayContract } from "./gateway-client-contract.mts";
import {
  defaultPackagePaths,
  providerWireRequiredPackages,
  validateBaseline,
  validateBaselineFiles,
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

describe("Gateway client contract witness", () => {
  const evidence = readGatewayContractEvidence();
  const baseline = { upstream: { commit: evidence.upstreamCommit }, packages: evidence.packages };

  it("matches the reviewed complete request contract and classifies all finite arms", () => {
    assert.deepEqual(validateGatewayContract(baseline, evidence, collectGatewayContract()), []);
  });

  it("fails baseline validation when all package consumers move but client evidence does not", () => {
    const directory = mkdtempSync(join(tmpdir(), "gateway-evidence-pin-"));
    try {
      const packages = { ...evidence.packages, "@ai-sdk/gateway": "4.0.53" };
      writeFileSync(join(directory, "baseline.json"), JSON.stringify({ ...baseline, packages }));
      writeFileSync(join(directory, "package.json"), JSON.stringify({ dependencies: packages }));
      const errors = validateBaselineFiles(join(directory, "baseline.json"), [join(directory, "package.json")]);
      assert.equal(errors.length, 1);
      assert.match(errors[0], /Gateway client evidence @ai-sdk\/gateway pin changed/);
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });

  it("rejects source commit changes independently of package pins", () => {
    assert.match(validateGatewayContract({ ...baseline, upstream: { commit: "new-commit" } }, evidence, evidence)[0], /upstream commit changed/);
  });

  it("detects fields and finite arms added to real parsed Go declarations", () => {
    const source = `package provider
type CallOptions struct { Nested *Nested; Reasoning ReasoningEffort }
type Nested struct { Text string }
type ReasoningEffort string
const ReasoningLow ReasoningEffort = "low"
`;
    const directory = mkdtempSync(join(tmpdir(), "gateway-contract-mutation-"));
    try {
      const path = join(directory, "contract.go");
      writeFileSync(path, source);
      const original = collectGatewayContract(directory);
      const reviewed = { ...evidence, ...original, classifications: { ReasoningLow: "mapped" as const } };
      const mutations = [
        { name: "CallOptions field", source: source.replace("Nested *Nested;", "Future string; Nested *Nested;"), error: /provider.CallOptions declaration changed/ },
        { name: "nested field", source: source.replace("Text string", "Text string; Future bool"), error: /provider.Nested declaration changed/ },
        { name: "typed discriminator", source: source + 'const ReasoningFuture ReasoningEffort = "future"\n', error: /discriminator ReasoningFuture changed/ },
        { name: "inherited discriminator", source: source + 'const ( ReasoningFuture ReasoningEffort = "future"; ReasoningFutureAlias )\n', error: /discriminator ReasoningFutureAlias changed/ },
        { name: "converted discriminator", source: source + 'const ReasoningFuture = ReasoningEffort("future")\n', error: /discriminator ReasoningFuture changed/ },
        { name: "aliased discriminator", source: source + 'const ReasoningFuture = ReasoningLow\n', error: /discriminator ReasoningFuture changed/ },
        { name: "parenthesized discriminator", source: source + 'const ReasoningFuture = (ReasoningLow)\n', error: /discriminator ReasoningFuture changed/ },
        { name: "concatenated discriminator", source: source + 'const ReasoningFuture = "future-" + ReasoningLow\n', error: /discriminator ReasoningFuture changed/ },
        { name: "type alias discriminator", source: source + 'type ReasoningAlias = ReasoningEffort\nconst ReasoningFuture ReasoningAlias = "future"\n', error: /discriminator ReasoningFuture changed/ },
        { name: "changed discriminator value", source: source.replace('= "low"', '= "changed"'), error: /discriminator ReasoningLow changed/ },
      ];
      for (const mutation of mutations) {
        writeFileSync(path, mutation.source);
        const errors = validateGatewayContract(baseline, reviewed, collectGatewayContract(directory));
        assert.ok(errors.some(error => mutation.error.test(error)), `${mutation.name}: ${errors.join("; ")}`);
      }
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });

  it("rejects newly recorded arms until their mapping is classified", () => {
    const updated = structuredClone(evidence);
    updated.constants.ReasoningEffort.ReasoningFuture = '"future"';
    assert.match(validateGatewayContract(baseline, updated, updated)[0], /ReasoningFuture has no mapping classification/);
  });

  it("fails closed when the provider source is missing", () => {
    const directory = mkdtempSync(join(tmpdir(), "gateway-contract-missing-"));
    try {
      assert.throws(() => collectGatewayContract(directory), /provider source package not found/);
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });
});
