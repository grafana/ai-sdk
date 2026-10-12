import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";
import { createGateway } from "@ai-sdk/gateway";
import { createCaptureFetch, drainStream } from "./capture.ts";
import {
  comprehensiveGoldenCase,
  fileInputGoldenCase,
  headersGoldenCase,
  providerToolsGoldenCase,
  mcpToolsGoldenCase,
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

  it("captures provider tools and assistant continuation", async () => {
    const [unary, streaming] = await providerToolsGoldenCase.capture();
    assert.equal(unary.streaming, false);
    assert.equal(streaming.streaming, true);
    const first = unary.body as JsonObject;
    assert.deepEqual(first.tools.map((tool: JsonObject) => [tool.type, tool.id, tool.name]), [
      ["provider", "anthropic.code_execution_20260120", "python"],
      ["provider", "provider.search", "search"],
    ]);
    assert.deepEqual(first.tools[0].args, {});
    assert.deepEqual(first.tools[1].args, { limit: 0, enabled: false, nested: { value: null } });
    assert.equal(first.providerOptions, undefined);
    const assistant = message(streaming.body as JsonObject, "assistant");
    assert.deepEqual(assistant.content.map((part: JsonObject) => [part.type, part.toolCallId]), [
      ["tool-call", "call-code"], ["tool-result", "call-code"],
    ]);
    assert.equal(assistant.content[0].providerExecuted, true);
    assert.equal(assistant.content[0].toolName, "python");
    assert.deepEqual(assistant.content[1].output, { type: "json", value: { stdout: "0" } });
  });

  it("captures request-level MCP servers and assistant continuation without definitions", async () => {
    const [unary, streaming] = await mcpToolsGoldenCase.capture();
    assert.equal(unary.streaming, false);
    assert.equal(streaming.streaming, true);
    const first = unary.body as JsonObject;
    assert.equal(first.tools, undefined);
    const server = first.providerOptions.anthropic.mcpServers[0];
    assert.deepEqual(server.toolConfiguration, { enabled: false, allowedTools: [] });
    assert.equal(server.authorizationToken, "contract-dummy-token");
    const assistant = message(streaming.body as JsonObject, "assistant");
    assert.deepEqual(assistant.content.map((part: JsonObject) => [part.type, part.toolCallId]), [
      ["tool-call", "call-mcp"], ["tool-result", "call-mcp"],
    ]);
    assert.equal(assistant.content[0].providerExecuted, true);
    assert.deepEqual(assistant.content[0].providerOptions, { anthropic: { type: "mcp-tool-use", serverName: "weather" } });
    assert.deepEqual(assistant.content[1].providerOptions, assistant.content[0].providerOptions);
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

  it("preserves selected file arms and filename presence in both modes", async () => {
    const requests = await fileInputGoldenCase.capture();
    assert.deepEqual(requests.map((request) => request.streaming), [false, true]);
    for (const request of requests) {
      const body = request.body as JsonObject;
      const user = message(body, "user");
      const assistant = message(body, "assistant");
      const tool = message(body, "tool");
      const nestedFiles = tool.content[0].output.value;
      for (const files of [user.content, nestedFiles]) {
        assert.deepEqual(files.map((part: JsonObject) => part.data.type), ["data", "data", "url", "reference", "text"]);
      }
      assert.deepEqual(assistant.content.slice(0, 4).map((part: JsonObject) => part.data.type), ["data", "url", "reference", "text"]);
      assert.equal(user.content[0].data.data, "AAEC");
      assert.equal(nestedFiles[0].data.data, "BQY=");
      for (const files of [user.content, nestedFiles]) {
        assert.equal(files[1].data.data, "");
        assert.equal(files[1].filename, "");
        assert.equal("filename" in files[2], false);
        assert.deepEqual(files[3].data.reference, { provider: files === user.content ? "file-1" : "file-3" });
        assert.equal(files[4].data.text, "");
        assert.equal(files[4].filename, "");
      }
      assert.equal(assistant.content[1].filename, "");
      assert.equal(assistant.content[0].data.data, "YWxyZWFkeS1iYXNlNjQ=");
      assert.equal(body.prompt[0].providerOptions.provider.nested.nullValue, null);
      assert.equal(user.content[0].providerOptions.provider.nested.falseValue, false);
      assert.deepEqual(assistant.providerOptions.provider, {});
      assert.equal(nestedFiles[0].providerOptions.provider.nested.zero, 0);
    }
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
