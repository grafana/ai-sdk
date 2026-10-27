import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import { childEnvironment } from './authorize.mjs';
import { modulePath, SHA } from './merged-candidates.mjs';
import { goEnvironment } from './requirements.mjs';

const tooling = resolve(fileURLToPath(new URL('../..', import.meta.url)));

function run(command, args, options = {}) {
  return execFileSync(command, args, { encoding: 'utf8', maxBuffer: 64 << 20, stdio: ['ignore', 'pipe', 'pipe'], ...options });
}

export function candidateChecksums(candidate, { root = tooling, exec = run } = {}) {
  assert.match(candidate.mergeSha, SHA);
  const out = mkdtempSync(join(tmpdir(), 'published-archive-'));
  try {
    const version = `v${candidate.version}`;
    const result = JSON.parse(exec('go', ['-C', join(root, 'release/candidate'), 'run', './cmd/candidate', 'proxy',
      '-repo', root, '-rev', candidate.mergeSha, '-out', out, `${candidate.component}@${version}`], { env: goEnvironment() }));
    const sums = result.Checksums?.[`${modulePath(candidate.component)}@${version}`];
    assert.ok(sums?.Zip && sums?.GoMod, `no candidate checksums for ${candidate.tag}`);
    return sums;
  } finally {
    rmSync(out, { recursive: true, force: true });
  }
}

export async function verifyPublishedVersion(candidate, { root = tooling, exec = run, attempts = 10,
  wait = () => sleep(30_000) } = {}) {
  const expected = candidateChecksums(candidate, { root, exec });
  const target = `${modulePath(candidate.component)}@v${candidate.version}`;
  let failure;
  for (let attempt = 0; attempt < attempts; attempt++) {
    const work = mkdtempSync(join(tmpdir(), 'published-download-'));
    try {
      const download = JSON.parse(exec('go', ['mod', 'download', '-json', target], { cwd: work, env: {
        ...childEnvironment(), GOFLAGS: '-modcacherw', GOMODCACHE: join(work, 'cache'), GOPATH: join(work, 'gopath'),
        GOPROXY: 'https://proxy.golang.org', GOSUMDB: 'sum.golang.org', GONOSUMDB: '', GOPRIVATE: '', GONOPROXY: '' } }));
      if (download.Sum !== expected.Zip || download.GoModSum !== expected.GoMod) {
        throw Object.assign(new Error(`public proxy checksum of ${target} differs from its recorded candidate checksum`),
          { terminal: true });
      }
      return { target, ...expected };
    } catch (error) {
      if (error.terminal) throw error;
      failure = error.stderr?.toString().trim() || error.message;
    } finally {
      rmSync(work, { recursive: true, force: true });
    }
    if (attempt < attempts - 1) await wait();
  }
  throw new Error(`published version ${target} is unavailable from the public proxy: ${failure}`);
}
