// Replays recorded OpenAI Responses inputs to the official JavaScript SDK,
// directly and through the Go OpenAI provider and the Responses codec.
import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createInterface } from 'node:readline';
import { test } from 'node:test';
import OpenAI from 'openai';

const gateway = resolve(import.meta.dirname, '../..');
const recordings = resolve(gateway, '../test/conformance/openai/recorded');

type Item = { type: string; text?: string; name?: string; call_id?: string; arguments?: string; summary?: string[] };
type Outcome = { event: string; incomplete?: string; items: Item[]; usage: unknown };

function normalizeItem(o: any): Item {
  const item: Item = { type: o.type };
  if (o.type === 'message') {
    item.text = (o.content ?? [])
      .filter((c: any) => c.type === 'output_text')
      .map((c: any) => c.text)
      .join('');
  }
  if (o.type === 'function_call') {
    item.name = o.name;
    item.call_id = o.call_id;
    item.arguments = o.arguments;
  }
  if (o.type === 'reasoning') item.summary = (o.summary ?? []).map((s: any) => s.text);
  return item;
}

function normalizeUsage(u: any) {
  if (!u) return null;
  return {
    input_tokens: u.input_tokens,
    output_tokens: u.output_tokens,
    total_tokens: u.total_tokens,
    cached_tokens: u.input_tokens_details?.cached_tokens,
    reasoning_tokens: u.output_tokens_details?.reasoning_tokens,
  };
}

// run streams body through the SDK accumulator, which rejects events out of
// order, and checks that the items announced by output_item.done events match
// the terminal response.
async function run(baseURL: string, body: any): Promise<Outcome> {
  const client = new OpenAI({ apiKey: 'client-key', baseURL, maxRetries: 0 });
  const stream = client.responses.stream(body);
  const done: Item[] = [];
  let terminal = '';
  let sequence = -1;
  for await (const event of stream) {
    assert.ok(event.sequence_number > sequence, 'sequence numbers increase');
    sequence = event.sequence_number;
    if (event.type === 'response.output_item.done') done[event.output_index] = normalizeItem(event.item);
    if (event.type === 'response.completed' || event.type === 'response.incomplete' || event.type === 'response.failed') {
      assert.equal(terminal, '', 'one terminal event');
      terminal = event.type;
    }
  }
  const response = await stream.finalResponse();
  const items = response.output.map(normalizeItem);
  assert.deepEqual(done, items, 'done items match the terminal response');
  assert.notEqual(terminal, '', 'stream ends with a terminal event');
  return { event: terminal, incomplete: response.incomplete_details?.reason, items, usage: normalizeUsage(response.usage) };
}

test('official JavaScript SDK over recorded inputs: direct and through the codec', async (t) => {
  const directory = mkdtempSync(join(tmpdir(), 'responses-codec-js-'));
  const binary = join(directory, 'serve');
  execFileSync('go', ['build', '-race', '-mod=readonly', '-o', binary, './test/openai-responses-codec/cmd/serve'], {
    cwd: gateway,
    env: { ...process.env, GOWORK: 'off' },
    stdio: 'pipe',
  });
  const server = spawn(binary, [recordings], { stdio: ['ignore', 'pipe', 'inherit'] });
  try {
    const [line] = (await once(createInterface({ input: server.stdout! }), 'line')) as [string];
    const base = line.trim();
    const fixtures: { name: string; inputs: string[] }[] = await (await fetch(`${base}/fixtures`)).json();
    assert.ok(fixtures.length > 0);
    for (const fixture of fixtures) {
      const body = await (await fetch(`${base}/request/${fixture.name}`)).json();
      for (const input of fixture.inputs) {
        await t.test(`${fixture.name}/${input}`, async () => {
          const direct = await run(`${base}/direct/${fixture.name}/${input}/v1`, body);
          const codec = await run(`${base}/codec/${fixture.name}/${input}/v1`, body);
          assert.ok(direct.items.length > 0);
          assert.deepEqual(codec, direct);
        });
      }
    }
  } finally {
    server.kill();
    rmSync(directory, { recursive: true, force: true });
  }
});
