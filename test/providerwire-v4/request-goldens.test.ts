import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";
import { createGateway } from "@ai-sdk/gateway";
import { createCaptureFetch, drainStream } from "./capture.ts";
import {
  comprehensiveGoldenCase,
  headersGoldenCase,
  requestGoldenCases,
  scalarGoldenCase,
  sequenceGoldenCase,
  streamingGoldenCase,
  type RequestGoldenCase,
} from "./request-cases.ts";
import { assertValidRequest } from "./schema.ts";

type JsonObject = Record<string, any>;

function expectedGolden(testCase: RequestGoldenCase): unknown {
  return JSON.parse(
    readFileSync(new URL(`./goldens/${testCase.fileName}`, import.meta.url), "utf8"),
  );
}

function assertEnvelope(request: {
  method: string;
  path: string;
  headers: Record<string, string>;
  streaming: boolean;
}): void {
  assert.equal(request.method, "POST");
  assert.equal(request.path, "/language-model");
  assert.equal(request.headers["content-type"], "application/json");
  assert.equal(request.headers["ai-language-model-specification-version"], "4");
  assert.equal(request.headers["ai-language-model-streaming"], String(request.streaming));
  assert.equal(typeof request.headers["ai-language-model-id"], "string");
}

function message(body: JsonObject, role: string): JsonObject {
  const result = body.prompt.find((candidate: JsonObject) => candidate.role === role);
  if (result === undefined) {
    throw new Error(`missing ${role} message`);
  }
  return result;
}

function contentPart(
  content: JsonObject[],
  predicate: (candidate: JsonObject) => boolean,
  label: string,
): JsonObject {
  const result = content.find(predicate);
  if (result === undefined) {
    throw new Error(`missing ${label}`);
  }
  return result;
}

describe("registered Gateway semantic request goldens", () => {
  for (const testCase of requestGoldenCases) {
    it(`matches ${testCase.name}`, async () => {
      const actual = await testCase.capture();
      for (const [index, request] of actual.entries()) {
        assertEnvelope(request);
        assertValidRequest(request.body, `${testCase.name} request ${index + 1}`);
      }
      assert.deepEqual(actual, expectedGolden(testCase));
    });
  }

  it("preserves scalar presence and omission", async () => {
    const request = (await scalarGoldenCase.capture())[0];
    const body = request.body as JsonObject;
    assert.equal(body.maxOutputTokens, 0);
    assert.equal(body.temperature, 0);
    assert.equal(body.includeRawChunks, false);
    assert.equal(body.headers["x-contract-body"], "");
    assert.equal("x-contract-undefined" in body.headers, false);
    assert.deepEqual(body.stopSequences, []);
    assert.deepEqual(body.tools, []);
    assert.deepEqual(body.providerOptions.empty, {});
    assert.equal(body.providerOptions.opaque.nested.nullValue, null);
  });

  it("captures file transformations in every registered client position", async () => {
    const request = (await comprehensiveGoldenCase.capture())[0];
    const body = request.body as JsonObject;
    const userContent = message(body, "user").content as JsonObject[];
    const assistantContent = message(body, "assistant").content as JsonObject[];
    const inputBytes = contentPart(userContent, (part) => part.filename === "bytes.bin", "input bytes");
    const inputURL = contentPart(
      userContent,
      (part) => part.type === "file" && part.data?.type === "url",
      "input URL",
    );
    const reasoningBytes = contentPart(
      assistantContent,
      (part) => part.type === "reasoning-file" && part.data?.type === "data",
      "reasoning bytes",
    );
    const reasoningURL = contentPart(
      assistantContent,
      (part) => part.type === "reasoning-file" && part.data?.type === "url",
      "reasoning URL",
    );
    const contentResult = contentPart(
      assistantContent,
      (part) => part.type === "tool-result" && part.toolCallId === "call-content",
      "content tool result",
    );
    const resultBytes = contentPart(
      contentResult.output.value,
      (part) => part.filename === "result.bin",
      "result bytes",
    );
    const resultURL = contentPart(
      contentResult.output.value,
      (part) => part.type === "file" && part.data?.type === "url",
      "result URL",
    );

    assert.equal(inputBytes.data.data, "AAEC");
    assert.equal(inputURL.data.url, "https://example.com/%");
    assert.equal(reasoningBytes.data.data, "AwQ=");
    assert.equal(reasoningURL.data.url, "https://example.test/reasoning");
    assert.equal(resultBytes.data.data, "BQY=");
    assert.equal(resultURL.data.url, "https://example.test/result");
  });

  it("omits abortSignal from the streaming body", async () => {
    const request = (await streamingGoldenCase.capture())[0];
    assert.equal(request.streaming, true);
    assert.equal(request.hasSignal, true);
    assert.equal("abortSignal" in (request.body as object), false);
  });

  it("passes the exact abortSignal to fetch", async () => {
    const capture = createCaptureFetch();
    const controller = new AbortController();
    const model = createGateway({
      apiKey: "contract-test-key",
      baseURL: "https://contract.invalid",
      fetch: capture.fetch,
    })("grafana/signal");

    const result = await model.doStream({ prompt: [], abortSignal: controller.signal });
    await drainStream(result.stream);

    assert.equal(capture.signals[0], controller.signal);
  });

  it("records final case-insensitive header precedence", async () => {
    const requests = await headersGoldenCase.capture();
    assert.equal(requests[0].headers["ai-language-model-id"], "actual");
    assert.equal(requests[0].headers["x-contract-precedence"], "call");
    assert.equal(requests[0].headers["ai-o11y-deployment-id"], "deployment-1");
    assert.equal(requests[1].headers["ai-language-model-id"], "call");
    assert.equal((requests[1].body as JsonObject).headers["AI-Language-Model-Id"], "call");
  });

  it("ignores uncontrolled ambient observability headers", async () => {
    const previousEnvironment = process.env.VERCEL_ENV;
    process.env.VERCEL_ENV = "preview";
    try {
      for (const testCase of requestGoldenCases) {
        assert.deepEqual(await testCase.capture(), expectedGolden(testCase));
      }
    } finally {
      if (previousEnvironment === undefined) {
        delete process.env.VERCEL_ENV;
      } else {
        process.env.VERCEL_ENV = previousEnvironment;
      }
    }
  });

  it("preserves ordered unary and streaming calls", async () => {
    const requests = await sequenceGoldenCase.capture();
    assert.deepEqual(
      requests.map((request) => request.streaming),
      [false, true],
    );
    assert.deepEqual(
      requests.map((request) => (request.body as JsonObject).prompt[0].content[0].text),
      ["first", "second"],
    );
  });
});
