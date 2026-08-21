import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  compareContractTestSets,
  listedContractTests,
  resolvedContractTests,
} from "./contract-tests.ts";

describe("provider contract test evidence", () => {
  it("parses exactly one resolved contract test per delta row", () => {
    const tests = resolvedContractTests(
      "| `row-id` | distinction | evidence | loss | witness | change | `TestProviderWireV4Contract_RowID` (resolved) |",
    );
    assert.deepEqual([...tests], ["TestProviderWireV4Contract_RowID"]);
  });

  it("rejects a resolved row without one exact test", () => {
    assert.throws(
      () => resolvedContractTests("| `row-id` | (resolved) |"),
      /exactly one contract test/,
    );
  });

  it("parses only top-level contract tests from go list output", () => {
    const tests = listedContractTests(
      "TestProviderWireV4Contract_One\nTestOther\nok package\n",
    );
    assert.deepEqual([...tests], ["TestProviderWireV4Contract_One"]);
  });

  it("requires exact set equality", () => {
    assert.deepEqual(
      compareContractTestSets(new Set(["A", "B"]), new Set(["B", "C"])),
      ["missing provider contract test A", "unexpected provider contract test C"],
    );
  });
});
