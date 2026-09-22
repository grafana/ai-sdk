import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { createServer, type IncomingHttpHeaders } from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { after, before, describe, it } from "node:test";
import { createGateway } from "@ai-sdk/gateway";
import type { LanguageModelV4CallOptions } from "@ai-sdk/provider";
import { buildGoClientCapture, captureGoClient } from "./go-client-capture";
import { comprehensiveGoldenCase } from "./request-cases";
import { assertValidRequest } from "./schema";

let directory: string;
let binary: string;
before(() => { directory = mkdtempSync(join(tmpdir(), "wp7-go-differential-")); binary = buildGoClientCapture(directory, process.env.GRAFANA_CLIENT_MUTATION_SOURCE); });
after(() => { rmSync(directory, { recursive: true, force: true }); });

const unary = {
  content: [{ type: "text", text: "hello" }, { type: "text", text: "" }],
  finishReason: { unified: "stop", raw: "end_turn" },
  usage: { inputTokens: { total: 2, noCache: 2, cacheRead: 0, cacheWrite: 0 }, outputTokens: { total: 1, text: 1, reasoning: 0 } },
  request: { body: "ignored" }, response: { modelId: "ignored", id: "ignored" }, warnings: [{ type: "other", message: "ignored" }],
};

type Captured = { method: string; path: string; headers: IncomingHttpHeaders; body: unknown };
async function endpoint(body: string, status = 200, contentType = "application/json") {
  const requests: Captured[] = [];
  const server = createServer(async (request, response) => {
    const chunks: Buffer[] = [];
    for await (const chunk of request) chunks.push(Buffer.from(chunk));
    const raw = Buffer.concat(chunks).toString();
    requests.push({ method: request.method!, path: request.url!, headers: request.headers, body: raw === "" ? null : JSON.parse(raw) });
    response.writeHead(status, { "content-type": contentType, "x-public-response": "value" }); response.end(body);
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address(); assert.ok(address && typeof address !== "string");
  const baseURL = `http://127.0.0.1:${address.port}/api/v1/aisdk`;
  return { baseURL, requests, stop: () => new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve())) };
}

function semanticRequest(request: Captured) {
  if (request.method === "POST") assertValidRequest(request.body, "Go/TypeScript differential request");
  const names = ["x-access-token", "x-grafana-id", "x-configured", "x-call", "ai-language-model-id", "ai-language-model-specification-version", "ai-language-model-streaming", "content-type"];
  return { method: request.method, path: request.path, body: request.body, headers: Object.fromEntries(names.filter((name) => request.headers[name] !== undefined).map((name) => [name, request.headers[name]])) };
}

function goOptions(options: LanguageModelV4CallOptions): unknown {
  return JSON.parse(JSON.stringify(options, (_key, value: unknown) => value instanceof Uint8Array ? Buffer.from(value).toString("base64") : value));
}

async function cancellationEndpoint(streaming: boolean) {
  let requests = 0;
  let closed = 0;
  const server = createServer(async (request, response) => {
    for await (const _chunk of request) { /* consume request before observing cancellation */ }
    requests++;
    response.once("close", () => { closed++; });
    if (streaming) {
      response.writeHead(200, { "content-type": "text/event-stream" });
      response.write('data: {"type":"text-start","id":"first"}\n\n');
    }
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address(); assert.ok(address && typeof address !== "string");
  return {
    baseURL: `http://127.0.0.1:${address.port}/api/v1/aisdk`,
    requests: () => requests,
    closed: () => closed,
    stop: async () => { server.closeAllConnections(); await new Promise<void>((resolve) => server.close(() => resolve())); },
  };
}

function hasAbortCause(error: any): boolean {
  for (let current = error, depth = 0; current != null && depth < 10; current = current.cause, depth++) {
    if (current.name === "AbortError") return true;
  }
  return false;
}

async function eventually(check: () => boolean, label: string): Promise<void> {
  const deadline = Date.now() + 3_000;
  while (!check()) {
    assert.ok(Date.now() < deadline, label);
    await new Promise((resolve) => setTimeout(resolve, 5));
  }
}

describe("Go and exact-pinned Gateway differential", () => {
  it("covers every protected header with lower, upper, and canonical casing in both call modes", async () => {
    const protectedNames = ["x-access-token", "x-grafana-id", "content-type", "accept", "ai-language-model-id", "ai-language-model-specification-version", "ai-language-model-streaming"];
    const casings = [(name: string) => name, (name: string) => name.toUpperCase(), (name: string) => name.split("-").map((part) => part[0]!.toUpperCase() + part.slice(1)).join("-")];
    let classifiedOwnershipDifferences = 0;
    for (const streaming of [false, true]) {
      for (const header of protectedNames) for (const casing of casings) {
        const key = casing(header);
        const server = await endpoint(streaming ? 'data: {"type":"text-start","id":"a"}\n\n' : JSON.stringify(unary), 200, streaming ? "text/event-stream" : "application/json");
        try {
          const headers = { [key]: "call-injection", "x-custom": "call", "x-empty": "" };
          const options = { prompt: [], headers };
          const tsModel = createGateway({ apiKey: "test", baseURL: server.baseURL, headers: { "x-access-token": "token", "x-grafana-id": "user", "x-custom": "configured", [key]: "configured-injection" } })("assistant");
          const ts = streaming ? await tsModel.doStream(options) : await tsModel.doGenerate(options);
          if ("stream" in ts) for await (const _part of ts.stream) { /* drain the actual pinned client */ }
          const go = await captureGoClient(binary, { baseURL: server.baseURL, accessToken: "token", userIDToken: "user", modelID: "assistant", mode: streaming ? "stream" : "generate", headers: { [key]: ["configured-injection"], "x-custom": ["configured"] }, options });
          assert.equal(go.error, undefined);
          const actual = server.requests[1]!;
          const expected: Record<string, string> = { "x-access-token": "token", "x-grafana-id": "user", "content-type": "application/json", accept: streaming ? "text/event-stream" : "application/json", "ai-language-model-id": "assistant", "ai-language-model-specification-version": "4", "ai-language-model-streaming": String(streaming) };
          for (const name of protectedNames) assert.equal(actual.headers[name], expected[name], `${key}: ${name} must have one client-owned value`);
          assert.equal(actual.headers["x-custom"], "call");
          assert.equal(actual.headers["x-empty"], "");
          assert.deepEqual((actual.body as any).headers, headers);
          assert.deepEqual((ts.request?.body as any).headers, headers);
          assert.deepEqual((streaming ? go.request : go.result.request).body.headers, headers);
          if (server.requests[0]!.headers[header] !== actual.headers[header]) classifiedOwnershipDifferences++;
          assertValidRequest(actual.body, "protected header request");
        } finally { await server.stop(); }
      }
    }
    assert.ok(classifiedOwnershipDifferences > 0, "pinned permissive header overrides differ intentionally from protected Go ownership");
  });

  it("preserves cancellation before I/O, during unary reads, and after the first stream part", async () => {
    for (const streaming of [false, true]) {
      const server = await cancellationEndpoint(streaming);
      try {
        const client = createGateway({ apiKey: "test", baseURL: server.baseURL })("assistant");
        const options = { prompt: [], abortSignal: AbortSignal.abort() };
        await assert.rejects(Promise.resolve(streaming ? client.doStream(options) : client.doGenerate(options)), hasAbortCause);
        const before = await captureGoClient(binary, { baseURL: server.baseURL, accessToken: "token", modelID: "assistant", mode: streaming ? "stream" : "generate", options: { prompt: [] }, abortBefore: true });
        assert.equal(before.error?.canceled, true);
        assert.equal(server.requests(), 0);
        if (!streaming) {
          const controller = new AbortController();
          const pending = client.doGenerate({ prompt: [], abortSignal: controller.signal });
          await eventually(() => server.requests() === 1, "TypeScript unary entered transport");
          controller.abort();
          await assert.rejects(Promise.resolve(pending), hasAbortCause);
          const go = await captureGoClient(binary, { baseURL: server.baseURL, accessToken: "token", modelID: "assistant", mode: "generate", options: { prompt: [] }, cancelAfterMs: 100 });
          assert.equal(go.error?.canceled, true);
        } else {
          const controller = new AbortController();
          const result = await client.doStream({ prompt: [], abortSignal: controller.signal });
          const reader = result.stream.getReader();
          const first = await reader.read();
          controller.abort();
          await assert.rejects(reader.read(), hasAbortCause);
          const go = await captureGoClient(binary, { baseURL: server.baseURL, accessToken: "token", modelID: "assistant", mode: "stream", options: { prompt: [] }, abortAfterParts: 1 });
          assert.deepEqual(go.parts, [first.value]);
          assert.equal(go.canceled, true);
        }
        await eventually(() => server.closed() === 2, "both canceled transports close");
        assert.equal(server.requests(), 2, "cancellation must never trigger retries");
      } finally { await server.stop(); }
    }
  });

  it("projects the existing comprehensive pinned request golden with explicit Go presence gaps", async () => {
    const captured = (await comprehensiveGoldenCase.capture())[0]!;
    const original = captured.body as Record<string, any>;
    const expected = structuredClone(original);
    const assistant = expected.prompt.find((message: any) => message.role === "assistant");
    delete assistant.content.find((part: any) => part.type === "tool-call").providerExecuted;
    delete assistant.content.find((part: any) => part.toolCallId === "call-denied").output.reason;
    const tool = expected.prompt.find((message: any) => message.role === "tool");
    delete tool.content.find((part: any) => part.type === "tool-approval-response").reason;
    delete expected.responseFormat.name;
    delete expected.responseFormat.description;
    delete expected.tools[0].description;
    const server = await endpoint(JSON.stringify(unary));
    try {
      const go = await captureGoClient(binary, { baseURL: server.baseURL, accessToken: "token", modelID: "assistant", mode: "generate", options: original });
      assert.equal(go.error, undefined);
      assertValidRequest(server.requests[0]!.body, "Go comprehensive golden projection");
      assert.deepEqual(server.requests[0]!.body, expected);
    } finally { await server.stop(); }
  });

  const cases: Array<{ name: string; options: LanguageModelV4CallOptions }> = [
    { name: "text", options: { prompt: [{ role: "user", content: [{ type: "text", text: "hello" }] }] } },
    { name: "scalar and collection presence", options: { prompt: [{ role: "system", content: "" }, { role: "user", content: [{ type: "text", text: "" }] }], maxOutputTokens: 0, temperature: 0, topP: 0, topK: 0, presencePenalty: 0, frequencyPenalty: 0, seed: 0, stopSequences: [], tools: [], responseFormat: { type: "text" }, headers: { "x-call": "" }, providerOptions: { opaque: { nested: [null, false, 0, "", [], {}] }, empty: {} } } },
    { name: "file bytes URL reference and text", options: { prompt: [{ role: "user", content: [
      { type: "file", mediaType: "application/octet-stream", data: { type: "data", data: new Uint8Array([0, 1, 2]) } },
      { type: "file", mediaType: "text/plain", data: { type: "url", url: new URL("https://example.com/%") } },
      { type: "file", mediaType: "application/pdf", data: { type: "reference", reference: { provider: "file-1" } } },
      { type: "file", mediaType: "text/plain", data: { type: "text", text: "" } },
    ] }] } },
    { name: "tools schema and opaque options", options: { prompt: [{ role: "tool", content: [{ type: "tool-result", toolCallId: "call-1", toolName: "lookup", output: { type: "json", value: { ok: false } } }, { type: "tool-approval-response", approvalId: "approve-1", approved: false }] }], tools: [{ type: "function", name: "lookup", inputSchema: { type: "object", properties: {} }, strict: false, inputExamples: [] }], toolChoice: { type: "tool", toolName: "lookup" }, responseFormat: { type: "json", schema: { type: "object" }, name: "answer" }, reasoning: "high" } },
  ];
  for (const testCase of cases) it(`matches ${testCase.name} requests and unary normalization`, async () => {
    const server = await endpoint(JSON.stringify(unary));
    try {
      const configured = { "x-access-token": "access-token", "x-grafana-id": "acting-user", "x-configured": "configured" };
      const client = createGateway({ apiKey: "test-key", baseURL: server.baseURL, headers: configured });
      const requestOptions = testCase.name.includes("file") ? testCase.options : structuredClone(testCase.options);
      const input = goOptions(requestOptions);
      const ts = await client("assistant").doGenerate(requestOptions);
      const go = await captureGoClient(binary, { baseURL: server.baseURL, accessToken: "access-token", userIDToken: "acting-user", modelID: "assistant", mode: "generate", headers: { "x-configured": ["configured"] }, options: input, preferBytes: testCase.name.includes("file") });
      assert.equal(go.error, undefined);
      assert.deepEqual(semanticRequest(server.requests[1]!), semanticRequest(server.requests[0]!));
      assert.deepEqual(go.result.content.map((part: { type: string; text?: string }) => ({ type: part.type, text: part.text ?? "" })), ts.content);
      assert.deepEqual(go.result.finishReason, ts.finishReason); assert.deepEqual(go.result.usage, ts.usage);
      assert.deepEqual(go.result.request, JSON.parse(JSON.stringify(ts.request))); assert.equal(go.result.response.modelId, undefined); assert.equal(go.result.response.id, undefined);
      assert.deepEqual(ts.warnings, []); assert.equal(go.result.warnings, undefined);
    } finally { await server.stop(); }
  });

  it("matches public discovery and alias order", async () => {
    const models = [{ id: "assistant", name: "Assistant", description: null, specification: { specificationVersion: "v4", provider: "grafana", modelId: "assistant" } }, { id: "grafana/assistant", name: "Alias", specification: { specificationVersion: "v4", provider: "grafana", modelId: "grafana/assistant" } }];
    const server = await endpoint(JSON.stringify({ models, private: "ignored" }));
    try {
      const ts = await createGateway({ apiKey: "test", baseURL: server.baseURL, headers: { "x-access-token": "token" } }).getAvailableModels();
      const go = await captureGoClient(binary, { baseURL: server.baseURL, accessToken: "token", mode: "discovery" });
      assert.equal(go.error, undefined);
      assert.deepEqual(go.models.map((model: any) => ({ ...model, description: model.description ?? null })), ts.models.map((model) => ({ ...model, description: model.description ?? null })));
      assert.deepEqual(semanticRequest(server.requests[1]!), semanticRequest(server.requests[0]!));
    } finally { await server.stop(); }
  });

  it("matches stream order, raw filtering, timestamps, DONE and EOF", async () => {
    const values = [{ type: "stream-start", warnings: [] }, { type: "response-metadata", id: "response-1", modelId: "assistant", timestamp: "2026-08-22T00:00:00.123Z" }, { type: "text-start", id: "a" }, { type: "text-delta", id: "a", delta: "" }, { type: "raw", rawValue: { x: 1 } }, { type: "text-delta", id: "a", delta: "hello" }, { type: "text-end", id: "a" }, { type: "finish", finishReason: { unified: "stop" }, usage: { inputTokens: {}, outputTokens: {} } }];
    for (const includeRawChunks of [false, true]) {
      const server = await endpoint(values.map((value) => `data: ${JSON.stringify(value)}\r\n\r\n`).join("") + "data: [DONE]\r\n\r\n", 200, "text/event-stream");
      try {
        const options: LanguageModelV4CallOptions = { prompt: [], ...(includeRawChunks ? { includeRawChunks: true } : {}) };
        const ts = await createGateway({ apiKey: "test", baseURL: server.baseURL, headers: { "x-access-token": "token" } })("assistant").doStream(options);
        const parts: unknown[] = []; for await (const part of ts.stream) parts.push(part);
        const go = await captureGoClient(binary, { baseURL: server.baseURL, accessToken: "token", modelID: "assistant", mode: "stream", options });
        assert.equal(go.error, undefined);
        const normalize = (part: any) => ({ ...part, ...(part.type === "stream-start" ? { warnings: part.warnings ?? [] } : {}), ...(part.timestamp ? { timestamp: new Date(part.timestamp).toISOString() } : {}) });
        assert.deepEqual(go.parts.map(normalize), parts.map(normalize));
        assert.deepEqual(semanticRequest(server.requests[1]!), semanticRequest(server.requests[0]!));
      } finally { await server.stop(); }
    }
  });

  it("matches non-terminal public errors, bare-CR framing, and EOF without finish", async () => {
    const values = [{ type: "text-start", id: "a" }, { type: "error", error: { message: "safe failure", type: "internal_server_error", code: "upstream_error", param: null, statusCode: 502, retryable: true } }, { type: "text-delta", id: "a", delta: "after error" }];
    for (const delimiter of ["\n", "\r\n", "\r"]) {
      const wire = "\ufeff" + values.map((value) => `event: message${delimiter}id: ignored${delimiter}data: ${JSON.stringify(value)}${delimiter}${delimiter}`).join("") + `data: {"type":"text-end","id":"ignored-unterminated"}`;
      const server = await endpoint(wire, 200, "text/event-stream");
      try {
        const ts = await createGateway({ apiKey: "test", baseURL: server.baseURL })("assistant").doStream({ prompt: [] });
        const parts: unknown[] = []; for await (const part of ts.stream) parts.push(part);
        const go = await captureGoClient(binary, { baseURL: server.baseURL, accessToken: "token", modelID: "assistant", mode: "stream", options: { prompt: [] } });
        assert.equal(go.error, undefined);
        const normalize = (part: any) => part.type === "error" ? { type: part.type, message: part.error.message, statusCode: part.error.statusCode, retryable: part.error.isRetryable ?? part.error.retryable } : part;
        assert.deepEqual(go.parts.map(normalize), parts.map(normalize));
        assert.deepEqual(go.parts.map((part: any) => part.type), ["text-start", "error", "text-delta"]);
      } finally { await server.stop(); }
    }
  });

  it("matches all registered status categories and retryability", async () => {
    const cases = [[400,"invalid_request_error","invalid_request"],[401,"authentication_error","authentication_error"],[403,"forbidden","forbidden"],[404,"model_not_found","model_not_found"],[429,"rate_limit_exceeded","rate_limit_exceeded"],[424,"failed_dependency","failed_dependency"],[500,"internal_server_error","internal_error"],[502,"internal_server_error","upstream_error"],[503,"internal_server_error","overloaded"],[504,"internal_server_error","timeout"],[499,"internal_server_error","canceled"]] as const;
    for (const [status, type, code] of cases) {
      const server = await endpoint(JSON.stringify({ error: { message: "safe message", type, param: null, code } }), status);
      try {
        let ts: any;
        try { await createGateway({ apiKey: "test", baseURL: server.baseURL })("assistant").doGenerate({ prompt: [] }); } catch (error) { ts = error; }
        assert.ok(ts);
        const go = await captureGoClient(binary, { baseURL: server.baseURL, accessToken: "token", modelID: "assistant", mode: "generate", options: { prompt: [] } });
        assert.equal(go.error.category, ts.type); assert.equal(go.error.statusCode, ts.statusCode); assert.equal(go.error.isRetryable, ts.isRetryable);
      } finally { await server.stop(); }
    }
  });
});
