import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { invalidRequests, validRequests } from "./schema-cases.ts";
import { formatValidationErrors, validateRequest } from "./schema.ts";

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
