import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { createRequestBodyValidator } from "./validate-evidence.ts";

const validate = createRequestBodyValidator();

function validates(value: unknown): boolean {
  return validate(value) as boolean;
}

describe("ProviderWire V4 request JSON Schema", () => {
  it("accepts required empty prompt and opaque provider JSON", () => {
    assert.equal(
      validates({
        prompt: [],
        stopSequences: [],
        includeRawChunks: false,
        responseFormat: {
          type: "json",
          schema: { $defs: { value: { type: "string" } }, $ref: "#/$defs/value" },
        },
        tools: [
          {
            type: "function",
            name: "schema-tool",
            inputSchema: {
              $defs: { input: { type: "object" } },
              $ref: "#/$defs/input",
            },
          },
        ],
        providerOptions: { example: { nested: null, values: [0, false, ""] } },
      }),
      true,
    );
  });

  for (const testCase of [
    { name: "unknown root member", value: { prompt: [], unknown: true } },
    { name: "unknown message discriminator", value: { prompt: [{ role: "other", content: [] }] } },
    {
      name: "inactive response-format member",
      value: { prompt: [], responseFormat: { type: "text", name: "mixed" } },
    },
    {
      name: "role-incompatible approval content",
      value: {
        prompt: [
          {
            role: "assistant",
            content: [{ type: "tool-approval-response", approvalId: "a", approved: true }],
          },
        ],
      },
    },
    {
      name: "missing required empty text member",
      value: { prompt: [{ role: "user", content: [{ type: "text" }] }] },
    },
    {
      name: "invalid standardized null",
      value: { prompt: [], includeRawChunks: null },
    },
    {
      name: "provider tool providerOptions",
      value: {
        prompt: [],
        tools: [
          {
            type: "provider",
            id: "example.search",
            name: "search",
            args: {},
            providerOptions: {},
          },
        ],
      },
    },
    {
      name: "non-JSON-Schema function input",
      value: {
        prompt: [],
        tools: [{ type: "function", name: "invalid", inputSchema: 42 }],
      },
    },
    {
      name: "non-JSON-Schema response schema",
      value: { prompt: [], responseFormat: { type: "json", schema: "invalid" } },
    },
    {
      name: "provider reference type property",
      value: {
        prompt: [
          {
            role: "user",
            content: [
              {
                type: "file",
                mediaType: "text/plain",
                data: { type: "reference", reference: { type: "provider-id" } },
              },
            ],
          },
        ],
      },
    },
    {
      name: "mixed file-data arm",
      value: {
        prompt: [
          {
            role: "user",
            content: [
              {
                type: "file",
                mediaType: "text/plain",
                data: { type: "text", text: "value", url: "https://example.test" },
              },
            ],
          },
        ],
      },
    },
  ]) {
    it(`rejects ${testCase.name}`, () => {
      assert.equal(validates(testCase.value), false);
    });
  }
});
