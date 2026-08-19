import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import {
  artifactKind,
  assertArtifactMetadata,
  assertUnaryFixtureInventory,
  generationAuthority,
  makeNormalizedArtifact,
  policyProfile,
  projectH2UnaryResult,
  replaceNormalizedArtifact,
  sourceFixture,
  sourceFixtureKey,
} from "./providerwire-v4-unary.mts";

const syntheticRaw = {
  content: [
    {
      type: "text",
      text: "safe",
      providerMetadata: { provider: { secret: true } },
    },
    {
      type: "tool-result",
      toolCallId: "call-1",
      toolName: "tool",
      result: { providerMetadata: "opaque result value stays" },
      providerMetadata: { provider: { secret: true } },
    },
  ],
  finishReason: { unified: "stop", raw: "end" },
  usage: {
    inputTokens: { total: 1 },
    outputTokens: { total: 2 },
    raw: { backend: 3 },
  },
  providerMetadata: { provider: { secret: true } },
  request: { body: { secret: "request" } },
  response: {
    id: "safe-response",
    timestamp: "2026-01-02T03:04:05Z",
    modelId: "backend-model",
    provider: "backend-provider",
    headers: { authorization: "secret" },
    body: { secret: "response" },
  },
  warnings: [{ type: "other", message: "safe warning" }],
};

test("projectH2UnaryResult removes only H2 disclosure fields", () => {
  const projected = projectH2UnaryResult(syntheticRaw);
  assert.deepEqual(projected, {
    content: [
      { type: "text", text: "safe" },
      {
        type: "tool-result",
        toolCallId: "call-1",
        toolName: "tool",
        result: { providerMetadata: "opaque result value stays" },
      },
    ],
    finishReason: { unified: "stop", raw: "end" },
    usage: {
      inputTokens: { total: 1 },
      outputTokens: { total: 2 },
    },
    response: {
      id: "safe-response",
      timestamp: "2026-01-02T03:04:05Z",
    },
    warnings: [{ type: "other", message: "safe warning" }],
  });
  assert.deepEqual(syntheticRaw.content[0].providerMetadata, {
    provider: { secret: true },
  });
});

test("normalized artifact records independent derived provenance", () => {
  const artifact = makeNormalizedArtifact(
    projectH2UnaryResult(syntheticRaw),
    "fixture-sha256",
  );
  assert.equal(artifact.artifactKind, artifactKind);
  assert.equal(artifact.artifactKind, "derived-policy-projection");
  assert.equal(artifact.source.fixture, sourceFixture);
  assert.equal(artifact.source.upstreamKey, sourceFixtureKey);
  assert.equal(artifact.source.input, "input.response.json");
  assert.equal(artifact.source.inputSha256, "fixture-sha256");
  assert.equal(artifact.policyProfile, policyProfile);
  assert.equal(artifact.generationAuthority, generationAuthority);
  assert.equal(artifact.commands.check, "mise run check-providerwire-v4");
  assert.equal(artifact.commands.update, "mise run update-providerwire-v4-artifacts");
  assertArtifactMetadata(artifact, "fixture-sha256");
});

test("unary inventory selects the only provenance-valid generate fixture", () => {
  assert.deepEqual(assertUnaryFixtureInventory(), [sourceFixture]);
});

test("unexpected eligible unary fixture cannot escape classification", () => {
  const directory = process.env.TEST_TMPDIR ?? process.env.TMPDIR ?? "/tmp";
  const root = mkdtempSync(join(directory, "providerwire-v4-unary-inventory-"));
  const fixture = join(root, "provider", "recorded", "unexpected");
  mkdirSync(fixture, { recursive: true });
  writeFileSync(join(fixture, "config.yaml"), "operation: generate\nmodel: test\nprompt: test\n");
  writeFileSync(join(fixture, "input.response.json"), "{}\n");
  writeFileSync(join(fixture, "expected-generate.json"), "{}\n");
  writeFileSync(join(fixture, "expected-requests.jsonl"), "{}\n");
  try {
    assert.throws(
      () => assertUnaryFixtureInventory(root, {}),
      /unclassified unary fixtures: provider\/recorded\/unexpected/,
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("failed staged schema validation preserves the destination", () => {
  const directory = process.env.TEST_TMPDIR ?? process.env.TMPDIR ?? "/tmp";
  const destination = join(
    directory,
    `providerwire-v4-unary-preserve-${process.pid}-${Date.now()}.json`,
  );
  writeFileSync(destination, "preserved\n");
  try {
    const invalid = makeNormalizedArtifact({ content: [] }, "fixture-sha256");
    assert.throws(() => replaceNormalizedArtifact(invalid, destination));
    assert.equal(readFileSync(destination, "utf8"), "preserved\n");
  } finally {
    rmSync(destination, { force: true });
  }
});
