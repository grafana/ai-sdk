import assert from "node:assert/strict";
import { createServer } from "node:http";
import { describe, it } from "node:test";
import { createGateway } from "@ai-sdk/gateway";
import { fetchConfiguredModels, type ConfiguredRoute } from "../../examples/configured-discovery";

const baseURL = "https://configured.invalid/api/v1/aisdk";
const headers = { "x-access-token": "dummy-access-token" };

function fixture(gateway: unknown = configuredRoute()) {
  return { models: [{ id: "public", name: "sk-ordinary-display", specification: { specificationVersion: "v4", provider: "grafana", modelId: "public" }, gateway }] };
}
function configuredRoute(): ConfiguredRoute {
  return { aliases: ["alias"], primary: { providerInstance: "primary", provider: "anthropic", providerModelId: "native" }, fallbacks: [{ providerInstance: "backup", provider: "openai", providerModelId: "backup" }] };
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
      assert.equal(result.models[0].gateway?.fallbacks[0].providerInstance, "backup");
      assert.equal(calls, 1);
    }
  });

  it("accepts empty/unextended catalogs and ignores unrelated additions", async () => {
    assert.deepEqual(await read({ models: [] }), { models: [] });
    const rows = fixture().models.map(({ gateway: _gateway, ...row }) => row);
    assert.deepEqual(await read({ models: rows, future: "ignored" }), { models: rows });
    const document = fixture();
    const augmented = { future: "ignored", models: document.models.map(row => ({ ...row, future: { credential: "do-not-promote" }, gateway: { ...configuredRoute(), future: { credential: "do-not-promote" }, primary: { ...configuredRoute().primary, future: "ignored" }, fallbacks: configuredRoute().fallbacks.map(candidate => ({ ...candidate, future: "ignored" })) } })) };
    assert.deepEqual(await read(augmented), document);
    const nullDescription = { models: rows.map(row => ({ ...row, description: null })) };
    assert.deepEqual(await read(nullDescription), nullDescription);
  });

  it("leaves nonblank-string policy to the server", async () => {
    for (const name of ["", " ", "\u0085", "\u00a0"]) {
      const models = [{ id: "public", name, specification: { specificationVersion: "v4", provider: "grafana", modelId: "public" } }];
      assert.deepEqual(await read({ models }), { models });
    }
    const document = fixture();
    document.models[0].name = "\ufeff";
    assert.deepEqual(await read(document), document);
  });

  it("retains standard JSON Unicode strings without repairing identities", async () => {
    const fields = ["name", "description", "providerInstance", "provider", "providerModelId"] as const;
    for (const field of fields) {
      for (const value of ["\ud800", "\udc00", "\udc00\ud800", "\ud800x", "🚀\ud800", "🚀", "�", "\\ud800", "\\/"]) {
        const gateway = { aliases: [], primary: { providerInstance: "primary", provider: "anthropic", providerModelId: "native", ...(["providerInstance", "provider", "providerModelId"].includes(field) ? { [field]: value } : {}) }, fallbacks: [] };
        const row = { id: "public", name: "Model", description: "Description", specification: { specificationVersion: "v4", provider: "grafana", modelId: "public" }, gateway, ...(field === "name" || field === "description" ? { [field]: value } : {}) };
        const response = Response.json({ models: [row] });
        const pending = fetchConfiguredModels({ baseURL, headers, fetch: async () => response });
        assert.deepEqual(await pending, { models: [row] });
        assert.equal(response.body?.locked, false);
      }
    }
  });

  it("keeps malformed ignored additions and standard last-member semantics", async () => {
    const document = fixture({ ...configuredRoute(), future: "\ud800", primary: { ...configuredRoute().primary, future: "\udc00" }, fallbacks: configuredRoute().fallbacks.map(candidate => ({ ...candidate, future: "\udc00" })) });
    assert.deepEqual(await read({ ...document, future: "\ud800", models: document.models.map(row => ({ ...row, future: "\udc00" })) }), fixture());
    const body = JSON.stringify(fixture());
    for (const [replacement, expected] of [[`"providerModelId":"\\ud800","providerModelId":"native"`, "native"], [`"providerModelId":"native","providerModelId":"\\ud800"`, "\ud800"]] as const) {
      const modified = body.replaceAll(`"providerModelId":"native"`, replacement);
      assert.notEqual(modified, body);
      const pending = fetchConfiguredModels({ baseURL, headers, fetch: async () => new Response(modified, { headers: { "content-type": "application/json" } }) });
      const gateway = configuredRoute();
      gateway.primary.providerModelId = expected;
      assert.deepEqual(await pending, fixture(gateway));
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

  it("leaves public-ID grammar to the server", async () => {
    for (const id of ["bad id", "public\n", "public\r", "public\u2028", "public\u2029", "grafaná", "a" + "-".repeat(128)]) {
      const models = [{ id, name: "Model", specification: { specificationVersion: "v4", provider: "grafana", modelId: id } }];
      assert.deepEqual(await read({ models }), { models });
    }
  });

  it("leaves route consistency and uniqueness to the server", async () => {
    const route = configuredRoute();
    const doc = fixture();
    const gateways = [{ ...route, aliases: ["alias", "alias"] }, { ...route, aliases: ["public"] }, { ...route, aliases: [], fallbacks: [] }, { ...route, primary: { ...route.primary, providerInstance: " " } }, { ...route, fallbacks: [{ ...route.primary, provider: "different" }] }, { ...route, fallbacks: [route.fallbacks[0], route.fallbacks[0]] }];
    const documents = [{ models: [] }, { models: [...doc.models, doc.models[0]] }, { models: doc.models.map(row => ({ ...row, specification: { ...row.specification, specificationVersion: "other", provider: "other", modelId: "contradiction" } })) }, ...gateways.map(value => fixture(value))];
    for (const document of documents) assert.deepEqual(await read(document), document);
    const { gateway: _gateway, ...withoutGateway } = doc.models[0];
    assert.deepEqual(await read({ models: [{ ...withoutGateway, gateway: null }] }), { models: [withoutGateway] });
  });

  it("rejects JSON shape and type errors without returning a partial catalog", async () => {
    const route = configuredRoute();
    const doc = fixture();
    const badGateway: unknown[] = [[], {}, { ...route, aliases: null }, { ...route, aliases: [2] }, { ...route, primary: undefined }, { ...route, primary: null }, { ...route, primary: [] }, { ...route, primary: { providerInstance: "p", provider: "anthropic" } }, { ...route, primary: { ...route.primary, provider: null } }, { ...route, fallbacks: null }, { ...route, fallbacks: [null] }, { ...route, fallbacks: [{ providerInstance: "p", provider: "anthropic" }] }, { ...route, fallbacks: [{ ...route.fallbacks[0], providerModelId: 2 }] }];
    const invalid: unknown[] = [null, {}, { models: null }, { models: [null] }, { models: [doc.models[0], { ...doc.models[0], name: 2 }] }, { models: [doc.models[0], { ...doc.models[0], description: 2 }] }, ...badGateway.map(value => fixture(value))];
    for (const value of invalid) await assert.rejects(() => read(value), /configured discovery: invalid catalog/);
  });

  for (const dimension of ["rows", "aliases", "candidates", "name", "description", "providerInstance", "provider", "providerModelId"] as const) {
    it(`does not duplicate server policy for ${dimension}`, async () => {
      for (const extra of [0, 1]) {
        const gateway = configuredRoute();
        gateway.aliases = [];
        const models = [{ id: "public", name: "Model", specification: { specificationVersion: "v4", provider: "grafana", modelId: "public" }, gateway }];
        if (dimension === "rows") {
          const rows = Array.from({ length: 1024 + extra }, (_, i) => ({ id: `row-${i}`, name: "Model", specification: { specificationVersion: "v4", provider: "grafana", modelId: `row-${i}` } }));
          assert.deepEqual(await read({ models: rows }), { models: rows });
          continue;
        }
        if (dimension === "aliases") {
          gateway.aliases = Array.from({ length: 128 + extra }, (_, i) => `alias-${i}`);
        } else if (dimension === "candidates") gateway.fallbacks = Array.from({ length: 15 + extra }, (_, i) => ({ providerInstance: "p", provider: "anthropic", providerModelId: `native-${i}` }));
        else if (dimension === "name") models[0].name = "a".repeat(2048 + extra);
        else if (dimension === "description") Object.assign(models[0], { description: "a".repeat(2048 + extra) });
        else gateway.primary[dimension] = "é".repeat(1024) + "a".repeat(extra);
        assert.deepEqual(await read({ models }), { models });
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

  it("accepts large complete catalogs when the read allowance permits", async () => {
    const candidates = Array.from({ length: 16 }, (_, i) => ({ providerInstance: "instance", provider: "anthropic", providerModelId: `${i}-` + '"'.repeat(512) }));
    const gateway: ConfiguredRoute = { aliases: ["alias"], primary: candidates[0], fallbacks: candidates.slice(1) };
    const models = Array.from({ length: 65 }, (_, i) => { const id = `public-${i}`; return { id, name: "Model", specification: { specificationVersion: "v4", provider: "grafana", modelId: id }, gateway }; });
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
