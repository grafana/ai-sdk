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
    execution: {
      requestedModelId: "assistant",
      canonicalModelId: "grafana/assistant",
      attempts: [
        { provider: "anthropic", modelId: "primary", providerInstance: "account-a", outcome: "failed", error: { message: "overloaded", type: "capacity", code: "busy", statusCode: 503 } },
        { provider: "openai", modelId: "secondary", outcome: "selected" },
      ],
    },
    nativeMetadata: { execution: { native: true }, empty: {}, null: null, privateNativeField: "opaque" },
  };

  for (const [name, value] of [
    ["selected candidate", namespace],
    ["complete array beyond old cap", { ...namespace, execution: { ...namespace.execution, attempts: Array.from({ length: 32 }, (_, i) => ({ provider: "native", modelId: `model-${i}`, outcome: i === 31 ? "selected" : "failed" })) } }],
    ["canceled native failure", { execution: { ...namespace.execution, attempts: [{ provider: "native", modelId: "model", outcome: "canceled", error: { statusCode: 503 } }] } }],
    ["numeric code", { execution: { ...namespace.execution, attempts: [{ provider: "native", modelId: "model", outcome: "failed", error: { code: 42 } }] } }],
    ["missing diagnostics", { execution: { ...namespace.execution, attempts: [{ provider: "native", modelId: "model", outcome: "failed" }] } }],
  ] as const) {
    it(`accepts ${name}`, () => {
      assert.equal(validateGatewayMetadata(value), true, formatValidationErrors(validateGatewayMetadata.errors));
    });
  }

  for (const [name, execution] of [
    ["empty attempts", { ...namespace.execution, attempts: [] }],
    ["missing outcome", { ...namespace.execution, attempts: [{ provider: "native", modelId: "model" }] }],
    ["unknown outcome", { ...namespace.execution, attempts: [{ provider: "native", modelId: "model", outcome: "retried" }] }],
    ["selected index", { ...namespace.execution, selectedAttempt: 2 }],
    ["completion claim", { ...namespace.execution, attempts: [{ provider: "native", modelId: "model", outcome: "selected", completion: "completed" }] }],
    ["selected error history", { ...namespace.execution, attempts: [{ provider: "native", modelId: "model", outcome: "selected", error: { message: "stream error" } }] }],
    ["raw details", { ...namespace.execution, attempts: [{ provider: "native", modelId: "model", outcome: "failed", error: { details: {} } }] }],
    ["empty error", { ...namespace.execution, attempts: [{ provider: "native", modelId: "model", outcome: "failed", error: {} }] }],
    ["attempt disposition", { ...namespace.execution, attempts: { state: "over-limit", count: 17 } }],
  ] as const) {
    it(`rejects ${name}`, () => {
      assert.equal(validateGatewayMetadata({ ...namespace, execution }), false);
    });
  }
});
