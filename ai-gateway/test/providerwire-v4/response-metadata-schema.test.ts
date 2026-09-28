import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";
import Ajv2020 from "ajv/dist/2020.js";

function compileResponseSchema(filename: string) {
  const schema = JSON.parse(readFileSync(new URL(`../../providerwire/v4/schema/${filename}`, import.meta.url), "utf8"));
  return new Ajv2020({ strict: true, formats: { "date-time": true } }).compile(schema);
}

const stream = compileResponseSchema("stream_event.json");
const unary = compileResponseSchema("unary_success.json");
const result = (content: unknown) => ({ content: [content], finishReason: { unified: "stop" }, usage: { inputTokens: {}, outputTokens: {} } });

describe("closed ProviderWire response metadata schema", () => {
  it("accepts approved fields on eligible events and content", () => {
    const text = { type: "text-start", id: "msg_1", providerMetadata: { openai: { itemId: "msg_1" } } };
    const call = { type: "tool-call", toolCallId: "call", toolName: "weather", input: "{}", providerMetadata: { anthropic: { caller: { type: "direct" } } } };
    assert.equal(stream(text), true);
    assert.equal(stream(call), true);
    assert.equal(unary(result({ type: "text", text: "hi", providerMetadata: text.providerMetadata })), true);
    assert.equal(unary(result(call)), true);
  });

  it("rejects private keys, wrong placement, malformed approved shapes, and unsafe IDs", () => {
    for (const event of [
      { type: "text-start", id: "a", providerMetadata: { private: { secret: "key" } } },
      { type: "text-end", id: "a", providerMetadata: { openai: { itemId: "a", secret: "key" } } },
      { type: "text-delta", id: "a", delta: "", providerMetadata: { openai: { itemId: "a" } } },
      { type: "text-start", id: "a", providerMetadata: { openai: { itemId: "Bearer secret" } } },
      { type: "tool-call", toolCallId: "call", toolName: "weather", input: "{}", providerMetadata: { anthropic: { caller: { type: "tool" } } } },
    ]) assert.equal(stream(event), false, JSON.stringify(event));
    assert.equal(unary(result({ type: "text", text: "hi", providerMetadata: { anthropic: { caller: { type: "direct" } } } })), false);
    assert.equal(unary(result({ type: "tool-call", toolCallId: "call", toolName: "weather", input: "{}", providerMetadata: { anthropic: { caller: { type: "direct", secret: "key" } } } })), false);
  });
});
