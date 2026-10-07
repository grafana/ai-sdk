import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { createGateway } from "@ai-sdk/gateway";
import { createCaptureFetch, drainStream } from "./capture.ts";
import { assertValidRequest } from "./schema.ts";
import { redactGatewayRequestForCapture } from "../../examples/redact-byok.ts";

describe("registered Gateway BYOK projection", () => {
  it("redacts unfamiliar credential fields without rewriting application strings", () => {
    const body = { prompt: "gateway.byok dummy-application", providerOptions: { gateway: { byok: { openai: [{ unknown: { nested: "dummy-secret" } }] } }, openai: { store: false } } };
    const safe = JSON.stringify(redactGatewayRequestForCapture(body));
    assert.ok(!safe.includes("dummy-secret"));
    assert.ok(safe.includes("gateway.byok dummy-application"));
    assert.equal(body.providerOptions.gateway.byok.openai[0].unknown.nested, "dummy-secret");
  });
  it("omits malformed, oversized and cyclic capture values without raw fallback", () => {
    const cyclic: Record<string, unknown> = {};
    cyclic.self = cyclic;
    for (const body of [cyclic, '{"providerOptions":{"gateway":{"byok":"dummy-incomplete', { prompt: "x".repeat(65_537) }, { providerOptions: { gateway: new Uint8Array([1, 2]) } }]) {
      assert.deepEqual(redactGatewayRequestForCapture(body), { capture: "omitted" });
    }
  });
  for (const streaming of [false, true]) {
    it(`preserves provider maps, credential order and caller metadata (${streaming})`, async () => {
      const capture = createCaptureFetch({ additionalHeaderNames: ["authorization"] });
      const model = createGateway({
        apiKey: "123:dummy-cap",
        baseURL: "https://contract.invalid",
        fetch: capture.fetch,
      })("openai/model-not-in-catalog");
      const options = {
        prompt: [],
        providerOptions: {
          gateway: {
            byok: {
              anthropic: [{ apiKey: "dummy-anthropic" }],
              openai: [{ apiKey: "dummy-openai-first" }, { apiKey: "dummy-openai-second" }],
            },
          },
          openai: { store: false },
        },
      };
      const result = streaming ? await model.doStream(options) : await model.doGenerate(options);
      if ("stream" in result) await drainStream(result.stream);
      assert.equal(capture.requests.length, 1);
      const request = capture.requests[0];
      assertValidRequest(request.body, "BYOK request");
      assert.deepEqual(request.body, options);
      assert.equal(request.streaming, streaming);
      assert.equal(request.headers.authorization, "Bearer 123:dummy-cap");
      assert.equal(request.headers["ai-language-model-id"], "openai/model-not-in-catalog");
      assert.deepEqual(result.request?.body, options);
      for (const body of [result.request?.body, JSON.stringify(result.request?.body)]) {
        const safe = redactGatewayRequestForCapture(body);
        assert.deepEqual(safe, { ...options, providerOptions: { ...options.providerOptions, gateway: { byok: "[REDACTED]" } } });
        assert.equal(JSON.stringify(safe).includes("dummy-"), false);
      }
      assert.deepEqual(result.request?.body, options);
      assert.equal(JSON.stringify(request.body).includes("dummy-cap"), false);
    });

    it(`sends a private access JWT as bearer without BYOK (${streaming})`, async () => {
      const capture = createCaptureFetch({ additionalHeaderNames: ["authorization", "x-access-token"] });
      const model = createGateway({
        apiKey: "dummy.access.jwt",
        baseURL: "https://private.invalid",
        fetch: capture.fetch,
      })("configured-model");
      const result = streaming ? await model.doStream({ prompt: [] }) : await model.doGenerate({ prompt: [] });
      if ("stream" in result) await drainStream(result.stream);
      assert.equal(capture.requests.length, 1);
      assert.equal(capture.requests[0].headers.authorization, "Bearer dummy.access.jwt");
      assert.equal(capture.requests[0].headers["x-access-token"], undefined);
      assert.deepEqual(capture.requests[0].body, { prompt: [] });
    });
  }
});
