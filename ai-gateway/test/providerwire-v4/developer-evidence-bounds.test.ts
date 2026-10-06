import assert from "node:assert/strict";
import { describe, it } from "node:test";

const sourceBytes = 1_048_576;
const componentBytes = 131_072;
const errorDiagnosticBytes = 24_576;
const successDiagnosticBytes = 262_144;
const essentialErrorBytes = 32_768;
const errorEnvelopeBytes = 65_536;
const attemptCount = 16;

type Diagnostic = { state: "available"; value: unknown } | { state: "malformed" | "over-limit" | "unavailable" | "redacted"; reason: string };

function jsonBodyWitness(source: string, budget: number): Diagnostic {
  if (source.length > sourceBytes || Buffer.byteLength(source) > sourceBytes) return { state: "over-limit", reason: "sourceBytes" };
  let value: unknown;
  try { value = JSON.parse(source); } catch { return { state: "malformed", reason: "invalidJSON" }; }
  const available: Diagnostic = { state: "available", value };
  return Buffer.byteLength(JSON.stringify(available)) <= budget ? available : { state: "over-limit", reason: "encodedBytes" };
}

describe("#321 proposed budget/disposition witnesses (not a production projector)", () => {
  it("component and error allocations accept exact bytes, reject one extra and count escaping", () => {
    for (const budget of [componentBytes, errorDiagnosticBytes, successDiagnosticBytes]) {
      const overhead = Buffer.byteLength(JSON.stringify({ state: "available", value: "" }));
      const exact = JSON.stringify("a".repeat(budget - overhead));
      const accepted = jsonBodyWitness(exact, budget);
      assert.equal(accepted.state, "available");
      assert.equal(Buffer.byteLength(JSON.stringify(accepted)), budget);
      assert.deepEqual(jsonBodyWitness(JSON.stringify("a".repeat(budget - overhead + 1)), budget), { state: "over-limit", reason: "encodedBytes" });
      assert.equal(jsonBodyWitness(JSON.stringify("\u0000".repeat(budget - overhead)), budget).state, "over-limit");
    }
    assert.deepEqual(jsonBodyWitness("{".repeat(sourceBytes + 1), componentBytes), { state: "over-limit", reason: "sourceBytes" });
    assert.deepEqual(jsonBodyWitness("{", componentBytes), { state: "malformed", reason: "invalidJSON" });
    assert.deepEqual(jsonBodyWitness("é".repeat(sourceBytes), componentBytes), { state: "over-limit", reason: "sourceBytes" });
  });

  it("valid empty/nested values and explicit dispositions remain complete JSON", () => {
    for (const value of [null, {}, [], "", false, 0, { nested: [null, false, 0, "", {}, []] }]) {
      assert.deepEqual(jsonBodyWitness(JSON.stringify(value), componentBytes), { state: "available", value });
    }
    const states: Diagnostic[] = [
      { state: "unavailable", reason: "producerDoesNotExpose" },
      { state: "redacted", reason: "credentialSource" },
      { state: "malformed", reason: "invalidJSON" },
      { state: "over-limit", reason: "encodedBytes" },
    ];
    assert.deepEqual(JSON.parse(JSON.stringify(states)), states);
    assert.equal("requestHeaders" in {}, false);
  });

  it("16 compact heterogeneous failure summaries leave error envelope headroom", () => {
    const attempts = Array.from({ length: attemptCount }, (_, index) => ({
      index: index + 1, provider: index % 2 ? "openai" : "anthropic", modelId: "native-model",
      selection: "failed", nativeError: { message: "denied", statusCode: index % 2 ? 429 : 401, isRetryable: Boolean(index % 2) },
    }));
    const essential = { error: { message: "all candidates failed", type: "failed_dependency", code: "failed_dependency", param: null }, providerMetadata: { gateway: { evidence: { attempts } } } };
    assert.ok(Buffer.byteLength(JSON.stringify(essential)) < essentialErrorBytes);
    assert.ok(essentialErrorBytes + errorDiagnosticBytes + 8192 <= errorEnvelopeBytes);
    assert.ok(successDiagnosticBytes + essentialErrorBytes < 1_048_576);
    assert.ok(Buffer.byteLength(JSON.stringify({ state: "over-limit", reason: "aggregateBytes" })) < 128);
    const overflow = { state: "over-limit", reason: "attemptCount", count: attemptCount + 1 };
    assert.deepEqual(JSON.parse(JSON.stringify(overflow)), overflow);
    assert.equal("value" in overflow, false);
  });

  it("source-specific safe representation retains ordinary token-looking values and topology", () => {
    const source = {
      requestHeaders: { Authorization: "dummy-provider-secret", "X-Api-Key": "dummy-key", Cookie: "dummy-session", "x-application": "sk-ordinary-value" },
      requestBody: { input: "sk-ordinary-prompt", providerOptions: { gateway: { byok: { openai: [{ apiKey: "dummy-byok-secret" }] } } } },
      configured: { provider: "anthropic-primary", modelId: "native-model", project: "authorized-project", apiKeyEnv: "SECRET_REFERENCE" },
    };
    const safe = {
      requestHeaders: { state: "available", value: { "x-application": source.requestHeaders["x-application"] }, redacted: true },
      requestBody: { state: "available", value: { input: source.requestBody.input }, redacted: true },
      configured: { provider: source.configured.provider, modelId: source.configured.modelId, project: source.configured.project },
    };
    const encoded = JSON.stringify(safe);
    for (const value of ["dummy-provider-secret", "dummy-key", "dummy-session", "dummy-byok-secret", "SECRET_REFERENCE"]) assert.equal(encoded.includes(value), false);
    for (const value of ["sk-ordinary-value", "sk-ordinary-prompt", "authorized-project", "anthropic-primary"]) assert.equal(encoded.includes(value), true);
    assert.equal(source.requestHeaders.Authorization, "dummy-provider-secret");
    assert.equal(source.requestBody.providerOptions.gateway.byok.openai[0].apiKey, "dummy-byok-secret");
  });
});
