import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { createGateway } from "@ai-sdk/gateway";
import { createCaptureFetch, drainStream } from "./capture.ts";
import { assertValidRequest } from "./schema.ts";

describe("registered Gateway BYOK projection", () => {
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
