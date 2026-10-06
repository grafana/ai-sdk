import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { invalidRequests, validRequests } from "./schema-cases.ts";
import { formatValidationErrors, validateRequest, validateStreamEvent, validateUnarySuccess } from "./schema.ts";

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
