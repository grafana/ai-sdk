import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { fixtureResponse, startReplay } from "./replay.mts";

test("replay isolates multi-step attempts and captures extra requests", async () => {
  const dir = mkdtempSync(join(tmpdir(), "replay-unit-"));
  try {
    writeFileSync(join(dir, "input-1.chunks.txt"), '{"type":"first"}\n');
    writeFileSync(join(dir, "input-2.chunks.txt"), '{"type":"second"}\n');
    const tc = { name: "unit", dir, provider: "anthropic" };
    const first = await startReplay(tc);
    const second = await startReplay(tc);
    const call = (url: string) => fetch(`${url}/v1/messages`, { method: "POST", headers: { "x-api-key": "secret" }, body: '{"model":"test"}' });
    try {
      assert.match(await (await call(first.url)).text(), /first/);
      assert.match(await (await call(first.url)).text(), /second/);
      assert.match(await (await call(second.url)).text(), /first/);
      assert.equal((await call(first.url)).status, 500);
      assert.equal(first.requests.length, 3);
      assert.equal(second.requests.length, 1);
      assert.equal(first.requests[0].path, "/v1/messages");
      assert.equal(first.requests[0].headers["x-api-key"], "<redacted>");
    } finally { await first.close(); await second.close(); }
    await assert.rejects(call(first.url));
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

test("replay frames SSE and Bedrock while retaining unary JSON", async () => {
  assert.equal(fixtureResponse('{"type":"text"}\n', "anthropic").toString(), 'event: text\ndata: {"type":"text"}\n\n');
  assert.equal(fixtureResponse('{"type":"text"\n', "openai").toString(), 'event: unknown\ndata: {"type":"text"\n\n');
  assert.equal(fixtureResponse('{}\n', "openai-compatible").toString(), 'event: unknown\ndata: {}\n\n');
  const frame = fixtureResponse('{"contentBlockDelta":{"delta":{"text":"hi"}}}\n', "bedrock");
  assert.equal(frame.readUInt32BE(0), frame.length);
  assert.match(frame.toString(), /contentBlockDelta/);
  const dir = mkdtempSync(join(tmpdir(), "replay-unary-"));
  try {
    writeFileSync(join(dir, "input.response.json"), '{"output":"unit"}');
    const replay = await startReplay({ name: "unit", dir, provider: "bedrock" });
    try {
      const response = await fetch(replay.url, { method: "POST", body: "{}" });
      assert.equal(response.headers.get("content-type"), "application/json");
      assert.deepEqual(await response.json(), { output: "unit" });
    } finally { await replay.close(); }
  } finally { rmSync(dir, { recursive: true, force: true }); }
});
