import assert from 'node:assert/strict';
import { pathToFileURL } from 'node:url';
import { GitHub } from 'release-please';
import { readOnlyFetch } from './preview.mjs';
import { REPO } from './merged-candidates.mjs';
import { GATEWAY_PUBLICATION_JOBS, successfulRun } from './authorization-evidence.mjs';
import { waitForToolingCI } from './activation.mjs';

export const DEV_PUBLICATION_WORKFLOW = 'publish-ai-gateway-dev.yml';
export const DEV_PUBLICATION_JOB = 'Publish AI Gateway plugin to the dev catalog / Publish to catalog (dev)';

function canonicalMain(env) {
  assert.equal(env.GITHUB_REPOSITORY, 'grafana/ai-sdk');
  assert.equal(env.GITHUB_REF, 'refs/heads/main');
}

export function publicationCommit(env) {
  canonicalMain(env);
  assert.equal(env.GITHUB_EVENT_NAME, 'push');
  assert.match(env.GITHUB_SHA ?? '', /^[0-9a-f]{40}$/);
  return env.GITHUB_SHA;
}

export function promotionCommit(env) {
  canonicalMain(env);
  assert.equal(env.GITHUB_EVENT_NAME, 'workflow_dispatch');
  assert.match(env.GATEWAY_PROMOTED_COMMIT ?? '', /^[0-9a-f]{40}$/, 'the promoted commit must be a full commit SHA');
  return env.GATEWAY_PROMOTED_COMMIT;
}

export async function requireSourceCI(github, sha, options = {}) {
  return waitForToolingCI(github, sha, { ...options, jobs: GATEWAY_PUBLICATION_JOBS });
}

export async function requireDevPublication(github, sha) {
  return successfulRun(github, DEV_PUBLICATION_WORKFLOW, 'push', sha, 'main', [DEV_PUBLICATION_JOB]);
}

export async function main(env = process.env) {
  const promotion = env.GITHUB_EVENT_NAME === 'workflow_dispatch';
  const sha = promotion ? promotionCommit(env) : publicationCommit(env);
  assert.ok(env.GH_READ_TOKEN, 'an Actions read credential is required');
  const github = await GitHub.create({ ...REPO, defaultBranch: 'main', token: env.GH_READ_TOKEN, fetch: readOnlyFetch() });
  if (!promotion) {
    console.log(JSON.stringify(await requireSourceCI(github, sha)));
    return;
  }
  const source = await requireSourceCI(github, sha, { attempts: 1 });
  const dev = await requireDevPublication(github, sha);
  console.log(JSON.stringify({ source, dev }));
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch(error => { console.error(error.message); process.exitCode = 1; });
}
