import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { invalidRequests, validRequests } from "./schema-cases.ts";
import { formatValidationErrors, validateGatewayMetadata, validateRequest, validateStreamEvent, validateUnarySuccess } from "./schema.ts";

describe("ProviderWire V4 request schema", () => {
  for (const testCase of validRequests) {
    it(`accepts ${testCase.name}`, () => {
      const before = structuredClone(testCase.value);
      assert.equal(
        validateRequest(testCase.value),
        true,
        formatValidationErrors(validateRequest.errors),
      );
      assert.deepEqual(testCase.value, before);
    });
  }

  for (const testCase of invalidRequests) {
    it(`rejects ${testCase.name}`, () => {
      assert.equal(validateRequest(testCase.value), false, testCase.name);
      assert.notEqual(formatValidationErrors(validateRequest.errors), "");
    });
  }
});

for (const [name, validate, document] of [
  ["unary", validateUnarySuccess, (timestamp: string) => ({
    content: [],
    finishReason: { unified: "stop" },
    usage: { inputTokens: {}, outputTokens: {} },
    warnings: [],
    response: { timestamp },
  })],
  ["streaming", validateStreamEvent, (timestamp: string) => ({ type: "response-metadata", timestamp })],
] as const) {
  describe(`ProviderWire V4 ${name} response timestamps`, () => {
    for (const timestamp of [
      "2026-09-30T12:00:00Z",
      "2024-02-29T00:00:00.123456789Z",
      "2026-09-30T12:00:00.123456789+01:00",
    ]) {
      it(`accepts ${timestamp}`, () => {
        assert.equal(validate(document(timestamp)), true, formatValidationErrors(validate.errors));
      });
    }
    for (const timestamp of [
      "2026-02-29T00:00:00Z",
      "2026-02-31T00:00:00Z",
      "2026-04-31T00:00:00Z",
      "2026-13-01T00:00:00Z",
    ]) {
      it(`rejects ${timestamp}`, () => {
        assert.equal(validate(document(timestamp)), false);
      });
    }
  });
}

describe("Gateway execution metadata schema", () => {
  const namespace = {
    routing: { originalModelId: "assistant", canonicalSlug: "grafana/assistant", finalProvider: "openai" },
    evidence: {
      requestedModelId: "assistant",
      canonicalModelId: "grafana/assistant",
      selectedAttempt: 2,
      attempts: [
        { index: 1, provider: "anthropic", modelId: "primary", selection: "failed", willFallback: true, nativeError: { statusCode: 429, isRetryable: true, details: { state: "available", value: null } } },
        { index: 2, provider: "openai", modelId: "secondary", selection: "selected", completion: "completed" },
      ],
    },
    nativeMetadata: { empty: {}, null: null, privateNativeField: "opaque" },
  };

  for (const [name, value] of [
    ["selected success", namespace],
    ["attempt overflow", { ...namespace, evidence: { ...namespace.evidence, selectedAttempt: 17, attempts: { state: "over-limit", reason: "attemptCount", count: 17 } } }],
    ["committed failure", { ...namespace, evidence: { ...namespace.evidence, gateway: { httpStatusCode: 200, classification: "failed_dependency", phase: "committed", isRetryable: false, replayRisk: "generationCompleted" } } }],
    ["partial redaction", { ...namespace, evidence: { ...namespace.evidence, attempts: [{ index: 1, provider: "native", modelId: "model", selection: "failed", nativeError: { isRetryable: false, details: { state: "available", value: {}, redacted: true } } }] } }],
    ...[
      ["unavailable", "producerDoesNotExpose"],
      ["redacted", "credentialSource"],
      ["malformed", "invalidJSON"],
      ["over-limit", "aggregateBytes"],
    ].map(([state, reason]) => [state, { ...namespace, evidence: { ...namespace.evidence, attempts: [{ index: 1, provider: "native", modelId: "model", selection: "failed", nativeError: { isRetryable: false, details: { state, reason } } }] } }]),
  ] as const) {
    it(`accepts ${name}`, () => {
      assert.equal(validateGatewayMetadata(value), true, formatValidationErrors(validateGatewayMetadata.errors));
    });
  }

  for (const [name, evidence] of [
    ["unrun configured candidates", { ...namespace.evidence, attempts: [{ index: 1, provider: "native", modelId: "model" }] }],
    ["too many partial attempts", { ...namespace.evidence, attempts: Array.from({ length: 17 }, (_, i) => ({ index: i + 1, provider: "native", modelId: "model", selection: "failed" })) }],
    ["wrong overflow reason", { ...namespace.evidence, attempts: { state: "over-limit", reason: "aggregateBytes", count: 17 } }],
    ["invalid selection", { ...namespace.evidence, selectedAttempt: 0 }],
    ["fabricated disposition", { ...namespace.evidence, attempts: [{ index: 1, provider: "native", modelId: "model", selection: "failed", nativeError: { isRetryable: false, details: { state: "available" } } }] }],
  ] as const) {
    it(`rejects ${name}`, () => {
      assert.equal(validateGatewayMetadata({ ...namespace, evidence }), false);
    });
  }
});
