import assert from 'node:assert/strict';
import { pathToFileURL } from 'node:url';
import { GitHub } from 'release-please';
import { readOnlyFetch } from './preview.mjs';
import { REPO } from './merged-candidates.mjs';
import { GATEWAY_PUBLICATION_JOBS } from './authorization-evidence.mjs';
import { waitForToolingCI } from './activation.mjs';

export function publicationCommit(env) {
  assert.equal(env.GITHUB_REPOSITORY, 'grafana/ai-sdk');
  assert.equal(env.GITHUB_EVENT_NAME, 'push');
  assert.equal(env.GITHUB_REF, 'refs/heads/main');
  assert.match(env.GITHUB_SHA ?? '', /^[0-9a-f]{40}$/);
  return env.GITHUB_SHA;
}

export async function requireSourceCI(github, sha, options = {}) {
  return waitForToolingCI(github, sha, { ...options, jobs: GATEWAY_PUBLICATION_JOBS });
}

export async function main(env = process.env) {
  const sha = publicationCommit(env);
  assert.ok(env.GH_READ_TOKEN, 'an Actions read credential is required');
  const github = await GitHub.create({ ...REPO, defaultBranch: 'main', token: env.GH_READ_TOKEN, fetch: readOnlyFetch() });
  console.log(JSON.stringify(await requireSourceCI(github, sha)));
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch(error => { console.error(error.message); process.exitCode = 1; });
}
