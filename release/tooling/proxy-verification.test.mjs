import assert from 'node:assert/strict';
import test from 'node:test';
import { verifyPublishedVersion } from './proxy-verification.mjs';

const candidate = { component: 'providers/openai', version: '0.1.0-alpha.2', tag: 'providers/openai/v0.1.0-alpha.2', mergeSha: 'c'.repeat(40) };
const target = 'github.com/grafana/ai-sdk/providers/openai@v0.1.0-alpha.2';
const sums = { Zip: 'h1:zip=', GoMod: 'h1:mod=' };

function executor(downloads) {
  const calls = [];
  const exec = (command, args, options) => {
    calls.push({ command, args, options });
    if (args.includes('proxy')) {
      assert.ok(args.includes(candidate.mergeSha));
      assert.equal(args.at(-1), 'providers/openai@v0.1.0-alpha.2');
      return JSON.stringify({ Checksums: { [target]: sums } });
    }
    assert.deepEqual(args, ['mod', 'download', '-json', target]);
    assert.equal(options.env.GOPROXY, 'https://proxy.golang.org');
    assert.equal(options.env.GOSUMDB, 'sum.golang.org');
    assert.equal(options.env.GONOSUMDB, '');
    const next = downloads.shift();
    if (next instanceof Error) throw next;
    return JSON.stringify(next);
  };
  return { exec, calls };
}

test('published version matches its candidate archive', async () => {
  const { exec, calls } = executor([{ Sum: sums.Zip, GoModSum: sums.GoMod }]);
  assert.deepEqual(await verifyPublishedVersion(candidate, { exec, wait: async () => assert.fail('no retry') }), { target, ...sums });
  assert.equal(calls.length, 2);
});

test('checksum mismatch fails without retry', async () => {
  const { exec, calls } = executor([{ Sum: 'h1:other=', GoModSum: sums.GoMod }]);
  await assert.rejects(verifyPublishedVersion(candidate, { exec, wait: async () => assert.fail('no retry') }), /differs from its recorded candidate checksum/);
  assert.equal(calls.length, 2);
});

test('an unavailable version is retried a bounded number of times', async () => {
  const unavailable = Object.assign(new Error('exit 1'), { stderr: 'not found: unknown revision' });
  const { exec } = executor([unavailable, { Sum: sums.Zip, GoModSum: sums.GoMod }]);
  let waits = 0;
  await verifyPublishedVersion(candidate, { exec, wait: async () => { waits++; } });
  assert.equal(waits, 1);
  const exhausted = executor([unavailable, unavailable, unavailable]);
  await assert.rejects(verifyPublishedVersion(candidate, { exec: exhausted.exec, attempts: 3, wait: async () => {} }),
    /unavailable from the public proxy: not found: unknown revision/);
});
