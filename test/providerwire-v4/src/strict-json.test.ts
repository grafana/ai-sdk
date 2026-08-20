import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { parseStrictJson } from "./strict-json.ts";

describe("parseStrictJson", () => {
  it("parses valid semantic JSON", () => {
    assert.deepEqual(parseStrictJson('{"empty":[],"zero":0,"false":false}'), {
      empty: [],
      zero: 0,
      false: false,
    });
  });

  for (const testCase of [
    { name: "duplicate members", source: '{"value":1,"value":2}', pattern: /duplicate JSON member/ },
    { name: "nested duplicate members", source: '{"nested":{"x":1,"x":2}}', pattern: /duplicate JSON member/ },
    { name: "comments", source: '{/* no */"x":1}', pattern: /invalid JSON syntax/ },
    { name: "trailing commas", source: '{"x":1,}', pattern: /invalid JSON syntax/ },
    { name: "invalid numbers", source: '{"x":01}', pattern: /invalid JSON syntax/ },
    { name: "trailing data", source: '{"x":1} false', pattern: /invalid JSON syntax/ },
  ]) {
    it(`rejects ${testCase.name}`, () => {
      assert.throws(() => parseStrictJson(testCase.source), testCase.pattern);
    });
  }
});
