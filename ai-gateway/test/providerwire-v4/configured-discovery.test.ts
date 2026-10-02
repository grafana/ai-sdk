import assert from "node:assert/strict";
import { createServer } from "node:http";
import { describe, it } from "node:test";
import { createGateway } from "@ai-sdk/gateway";
import { fetchConfiguredModels, type ConfiguredRoute } from "../../examples/configured-discovery";

const baseURL = "https://configured.invalid/api/v1/aisdk";
const headers = { "x-access-token": "dummy-access-token" };

function fixture(gateway: unknown = configuredRoute()) {
  return { models: ["alias", "public"].map(id => ({ id, name: "sk-ordinary-display", specification: { specificationVersion: "v4", provider: "grafana", modelId: id }, gateway })) };
}
function configuredRoute(): ConfiguredRoute {
  return { canonicalModelId: "public", aliases: ["alias"], candidates: [{ providerInstance: "primary", provider: "anthropic", modelId: "native" }, { providerInstance: "backup", provider: "openai", modelId: "backup" }] };
}
function read(value: unknown, options: { maxBytes?: number } = {}) {
  return fetchConfiguredModels({ baseURL, headers, fetch: async () => Response.json(value), ...options });
}

describe("configured discovery companion", () => {
  it("retains typed configured facts; pinned stock discovery still strips them", async () => {
    const document = fixture();
    const stock = await createGateway({ baseURL, apiKey: "dummy", fetch: async () => Response.json(document) }).getAvailableModels();
    assert.equal("gateway" in stock.models[0], false);
    for (const selected of [headers, { Authorization: "Bearer 42:dummy-cap" }]) {
      let calls = 0;
      const result = await fetchConfiguredModels({ baseURL: baseURL + "/", headers: selected, fetch: async (url, init) => {
        calls++;
        assert.equal(String(url), baseURL + "/config");
        assert.equal(init?.method, "GET");
        assert.equal(init?.redirect, "error");
        assert.equal(init?.credentials, "omit");
        assert.deepEqual(Object.fromEntries(new Headers(init?.headers)), Object.fromEntries(new Headers(selected)));
        return Response.json(document);
      } });
      assert.deepEqual(result, document);
      assert.equal(result.models[0].gateway?.candidates[1].providerInstance, "backup");
      assert.equal(calls, 1);
    }
  });

  it("accepts empty/unextended catalogs and ignores unrelated additions", async () => {
    assert.deepEqual(await read({ models: [] }), { models: [] });
    const rows = fixture().models.map(({ gateway: _gateway, ...row }) => row);
    assert.deepEqual(await read({ models: rows, future: "ignored" }), { models: rows });
    const document = fixture();
    const augmented = { future: "ignored", models: document.models.map(row => ({ ...row, future: { credential: "do-not-promote" }, gateway: { ...configuredRoute(), future: { credential: "do-not-promote" }, candidates: configuredRoute().candidates.map(candidate => ({ ...candidate, future: "ignored" })) } })) };
    assert.deepEqual(await read(augmented), document);
    const nullDescription = { models: rows.map(row => ({ ...row, description: null })) };
    assert.deepEqual(await read(nullDescription), nullDescription);
  });

  it("preserves nonblank opaque strings using the same Unicode whitespace boundary", async () => {
    for (const name of ["", " ", "\u0085", "\u00a0"]) {
      await assert.rejects(() => read({ models: [{ id: "public", name, specification: { specificationVersion: "v4", provider: "grafana", modelId: "public" } }] }));
    }
    const document = fixture();
    document.models[0].name = "\ufeff";
    assert.deepEqual(await read(document), document);
  });

  it("rejects unpaired surrogates in recognized strings without repairing identities", async () => {
    const fields = ["name", "description", "providerInstance", "provider", "modelId"] as const;
    for (const field of fields) {
      for (const [value, valid] of [["\ud800", false], ["\udc00", false], ["\udc00\ud800", false], ["\ud800x", false], ["🚀\ud800", false], ["🚀", true], ["�", true], ["\\ud800", true], ["\\/", true]] as const) {
        const gateway = { canonicalModelId: "public", aliases: [], candidates: [{ providerInstance: "primary", provider: "anthropic", modelId: "native", ...(["providerInstance", "provider", "modelId"].includes(field) ? { [field]: value } : {}) }] };
        const row = { id: "public", name: "Model", description: "Description", specification: { specificationVersion: "v4", provider: "grafana", modelId: "public" }, gateway, ...(field === "name" || field === "description" ? { [field]: value } : {}) };
        const response = Response.json({ models: [row] });
        const pending = fetchConfiguredModels({ baseURL, headers, fetch: async () => response });
        if (!valid) await assert.rejects(() => pending, /invalid catalog/);
        else assert.deepEqual(await pending, { models: [row] });
        assert.equal(response.body?.locked, false);
      }
    }
  });

  it("keeps malformed ignored additions and standard last-member semantics", async () => {
    const document = fixture({ ...configuredRoute(), future: "\ud800", candidates: configuredRoute().candidates.map(candidate => ({ ...candidate, future: "\udc00" })) });
    assert.deepEqual(await read({ ...document, future: "\ud800", models: document.models.map(row => ({ ...row, future: "\udc00" })) }), fixture());
    const body = JSON.stringify(fixture());
    for (const [replacement, valid] of [[`"modelId":"\\ud800","modelId":"native"`, true], [`"modelId":"native","modelId":"\\ud800"`, false]] as const) {
      const modified = body.replaceAll(`"modelId":"native"`, replacement);
      assert.notEqual(modified, body);
      const pending = fetchConfiguredModels({ baseURL, headers, fetch: async () => new Response(modified, { headers: { "content-type": "application/json" } }) });
      if (valid) assert.deepEqual(await pending, fixture()); else await assert.rejects(() => pending);
    }
  });

  it("copies fragmented stream bytes immediately, including reused and empty chunks", async () => {
    const document = fixture();
    document.models[0].name = "🚀";
    const encoded = new TextEncoder().encode(JSON.stringify(document));
    let index = 0;
    const shared = new Uint8Array(1);
    let empty = true;
    const stream = new ReadableStream<Uint8Array>({ pull(controller) {
      if (empty) { empty = false; controller.enqueue(new Uint8Array(0)); return; }
      empty = true;
      if (index === encoded.length) { controller.close(); return; }
      shared[0] = encoded[index++];
      controller.enqueue(shared);
    } }, { highWaterMark: 0 });
    assert.deepEqual(await fetchConfiguredModels({ baseURL, headers, maxBytes: encoded.length, fetch: async () => new Response(stream, { headers: { "content-type": "application/json" } }) }), document);
    assert.equal(stream.locked, false);
  });

  it("keeps the full public-ID grammar, including end-of-line rejection", async () => {
    for (const id of ["bad id", "public\n", "public\r", "public\u2028", "public\u2029", "grafaná", "a" + "-".repeat(128)]) {
      await assert.rejects(() => read({ models: [{ id, name: "Model", specification: { specificationVersion: "v4", provider: "grafana", modelId: id } }] }));
    }
    for (const alias of ["alias\n", "alias\u2028"]) {
      const gateway = { ...configuredRoute(), aliases: [alias] };
      await assert.rejects(() => read({ models: [gateway.canonicalModelId, alias].map(id => ({ id, name: "Model", specification: { specificationVersion: "v4", provider: "grafana", modelId: id }, gateway })) }));
    }
  });

  it("rejects malformed and inconsistent complete documents atomically", async () => {
    const route = configuredRoute();
    const doc = fixture();
    const badGateway: unknown[] = [null, [], {}, { ...route, canonicalModelId: "bad id" }, { ...route, aliases: null }, { ...route, aliases: ["alias", "alias"] }, { ...route, aliases: ["public"] }, { ...route, candidates: [] }, { ...route, candidates: null }, { ...route, candidates: [null] }, { ...route, candidates: [{ providerInstance: "p", provider: "anthropic" }] }, { ...route, candidates: [{ ...route.candidates[0], provider: null }] }, { ...route, candidates: [{ ...route.candidates[0], providerInstance: " " }] }, { ...route, candidates: [route.candidates[0], { ...route.candidates[0], provider: "different" }] }];
    const invalid: unknown[] = [null, {}, { models: null }, { models: [null] }, { models: [doc.models[0]] }, { models: [doc.models[1]] }, { models: [...doc.models, doc.models[0]] }, { models: [doc.models[0], { ...doc.models[1], gateway: { ...route, candidates: [route.candidates[0]] } }] }, { models: [{ ...doc.models[0], id: "undeclared", specification: { ...doc.models[0].specification, modelId: "undeclared" } }, doc.models[1]] }, { models: [doc.models[0], { ...doc.models[1], name: "" }] }, { models: [doc.models[0], { ...doc.models[1], description: 2 }] }, { models: doc.models.map(row => ({ ...row, specification: { ...row.specification, modelId: "contradiction" } })) }, ...badGateway.map(value => fixture(value))];
    for (const value of invalid) {
      await assert.rejects(() => read(value), /configured discovery: invalid catalog/);
    }
  });

  for (const dimension of ["rows", "aliases", "candidates", "name", "description", "providerInstance", "provider", "modelId"] as const) {
    it(`independently bounds ${dimension} at exact and one-over`, async () => {
      for (const extra of [0, 1]) {
        const gateway = configuredRoute();
        gateway.aliases = [];
        let models = [{ id: "public", name: "Model", specification: { specificationVersion: "v4", provider: "grafana", modelId: "public" }, gateway }];
        if (dimension === "rows") {
          const rows = Array.from({ length: 1024 + extra }, (_, i) => ({ id: `row-${i}`, name: "Model", specification: { specificationVersion: "v4", provider: "grafana", modelId: `row-${i}` } }));
          if (extra) await assert.rejects(() => read({ models: rows })); else assert.equal((await read({ models: rows })).models.length, 1024);
          continue;
        }
        if (dimension === "aliases") {
          gateway.aliases = Array.from({ length: 128 + extra }, (_, i) => `alias-${i}`);
          models = [models[0], ...gateway.aliases.map(id => ({ ...models[0], id, specification: { ...models[0].specification, modelId: id } }))];
        } else if (dimension === "candidates") gateway.candidates = Array.from({ length: 16 + extra }, (_, i) => ({ providerInstance: "p", provider: "anthropic", modelId: `native-${i}` }));
        else if (dimension === "name") models[0].name = "a".repeat(2048 + extra);
        else if (dimension === "description") Object.assign(models[0], { description: "a".repeat(2048 + extra) });
        else gateway.candidates[0][dimension] = "é".repeat(1024) + "a".repeat(extra);
        if (extra) await assert.rejects(() => read({ models })); else assert.equal((await read({ models })).models.length, models.length);
      }
    });
  }

  it("bounds encoded bytes including escaping without returning a partial catalog", async () => {
    const document = fixture();
    document.models[0].name = "quotes\" slash\\\u0000\n<>&";
    const bytes = Buffer.byteLength(JSON.stringify(document));
    assert.deepEqual(await read(document, { maxBytes: bytes }), document);
    await assert.rejects(() => read(document, { maxBytes: bytes - 1 }), /byte limit exceeded/);
    for (const maxBytes of [0, -1, 1.5, Number.MAX_SAFE_INTEGER, 4_194_305]) await assert.rejects(() => read(document, { maxBytes }), /invalid byte limit/);
    assert.deepEqual(await read({ models: [] }, { maxBytes: 4_194_304 }), { models: [] });
  });

  it("accepts within the independent 4 MiB allowance, not the server's 1 MiB default", async () => {
    const gateway: ConfiguredRoute = { canonicalModelId: "public", aliases: Array.from({ length: 64 }, (_, i) => `alias-${i}`), candidates: Array.from({ length: 16 }, (_, i) => ({ providerInstance: "instance", provider: "anthropic", modelId: `${i}-` + "a".repeat(1048) })) };
    const models = [gateway.canonicalModelId, ...gateway.aliases].map(id => ({ id, name: "Model", specification: { specificationVersion: "v4", provider: "grafana", modelId: id }, gateway }));
    const bytes = Buffer.byteLength(JSON.stringify({ models }));
    assert.ok(bytes > 1_048_576 && bytes < 4_194_304);
    assert.equal((await read({ models })).models.length, models.length);
    await assert.rejects(() => read({ models }, { maxBytes: 1_048_576 }), /byte limit exceeded/);
  });

  it("rejects unsafe URL, media, malformed JSON and UTF-8", async () => {
    for (const url of ["relative", "ftp://host", "https://user:password@host", baseURL + "?x=1", baseURL + "#x"]) {
      await assert.rejects(() => fetchConfiguredModels({ baseURL: url, headers, fetch: async () => { throw new Error("must not fetch"); } }), /invalid base URL/);
    }
    for (const response of [new Response("{}", { headers: { "content-type": "text/html" } }), new Response("{", { headers: { "content-type": "application/json" } }), new Response('{"models":[]} {}', { headers: { "content-type": "application/json" } }), new Response(new Uint8Array([123, 255, 125]), { headers: { "content-type": "application/json" } }), new Response("dummy credential body", { status: 403 })]) {
      await assert.rejects(() => fetchConfiguredModels({ baseURL, headers, fetch: async () => response }), error => error instanceof Error && !error.message.includes("dummy credential body"));
      assert.equal(response.body?.locked, false);
    }
  });

  it("cancels overflowing and aborted reads and releases the reader", async () => {
    for (const mode of ["overflow", "abort"] as const) {
      const controller = new AbortController();
      let canceled = false;
      const stream = new ReadableStream<Uint8Array>({ start(streamController) { if (mode === "overflow") streamController.enqueue(new Uint8Array(4096)); }, cancel() { canceled = true; } });
      const pending = fetchConfiguredModels({ baseURL, headers, signal: controller.signal, maxBytes: 64, fetch: async () => new Response(stream, { headers: { "content-type": "application/json" } }) });
      if (mode === "abort") queueMicrotask(() => controller.abort());
      await assert.rejects(() => pending);
      assert.equal(canceled, true);
      assert.equal(stream.locked, false);
    }
    const controller = new AbortController();
    controller.abort();
    let calls = 0;
    await assert.rejects(() => fetchConfiguredModels({ baseURL, headers, signal: controller.signal, fetch: async () => { calls++; return Response.json(fixture()); } }));
    assert.equal(calls, 0);
    const stream = new ReadableStream<Uint8Array>({ start(controller) { controller.error(new Error("dummy transport secret")); } });
    await assert.rejects(() => fetchConfiguredModels({ baseURL, headers, fetch: async () => new Response(stream, { headers: { "content-type": "application/json" } }) }), /configured discovery: read failed/);
    assert.equal(stream.locked, false);
  });

  it("refuses real redirects without forwarding credentials", async () => {
    let forwarded = 0;
    const target = createServer((_request, response) => { forwarded++; response.end(); });
    await new Promise<void>(resolve => target.listen(0, "127.0.0.1", resolve));
    const address = target.address();
    assert.ok(address && typeof address !== "string");
    const redirect = createServer((_request, response) => { response.writeHead(302, { location: `http://127.0.0.1:${address.port}/config` }); response.end(); });
    await new Promise<void>(resolve => redirect.listen(0, "127.0.0.1", resolve));
    const source = redirect.address();
    assert.ok(source && typeof source !== "string");
    try {
      await assert.rejects(() => fetchConfiguredModels({ baseURL: `http://127.0.0.1:${source.port}`, headers }));
      assert.equal(forwarded, 0);
    } finally {
      await Promise.all([new Promise<void>(resolve => redirect.close(() => resolve())), new Promise<void>(resolve => target.close(() => resolve()))]);
    }
  });
});
