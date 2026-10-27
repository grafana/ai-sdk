import assert from 'node:assert/strict';
import { REPO, REPOSITORY_ID, BOT_ID, SHA } from './merged-candidates.mjs';

export const SOURCE_JOBS = ['ci', 'race', 'docs-lint', 'module-resolution',
  'parity-baseline', 'integration-test', 'conformance-test'];

export function verifyMergedIdentity(candidate, head) {
  const { pr } = candidate;
  assert.equal(pr.number, candidate.prNumber);
  assert.equal(pr.state, 'closed');
  assert.equal(pr.merged, true);
  assert.equal(pr.draft, false);
  assert.equal(pr.user.id, BOT_ID);
  assert.equal(pr.user.login, 'grafana-ai-sdk-release[bot]');
  assert.equal(pr.user.type, 'Bot');
  for (const side of ['base', 'head']) {
    assert.equal(pr[side].repo.id, REPOSITORY_ID);
    assert.equal(pr[side].repo.full_name, 'grafana/ai-sdk');
  }
  assert.equal(pr.base.ref, 'main');
  assert.equal(pr.merge_commit_sha, candidate.mergeSha);
  assert.equal(pr.head.sha, candidate.headSha);
  for (const sha of [candidate.headSha, candidate.mergeSha, candidate.treeSha, ...candidate.parents]) assert.match(sha, SHA);
  assert.equal(candidate.parents.length, 1, 'only squash merges are supported');
  assert.equal(head.sha, candidate.headSha);
  assert.equal(head.author?.id, BOT_ID);
  assert.equal(head.commit.verification?.verified, true, 'release head must have a verified App signature');
  assert.equal(head.commit.verification.reason, 'valid');
}

export async function successfulRun(github, workflow, event, sha, branch, jobs) {
  const { data: definition } = await github.octokit.actions.getWorkflow({ ...REPO, workflow_id: workflow });
  const runs = await github.octokit.paginate(github.octokit.actions.listWorkflowRuns, {
    ...REPO, workflow_id: definition.id, event, head_sha: sha, per_page: 100,
  });
  const run = runs.filter(run => run.event === event && run.head_sha === sha && run.head_branch === branch)
    .sort((a, b) => b.id - a.id)[0];
  assert.ok(run, `missing ${workflow} at ${sha}`);
  assert.equal(run.workflow_id, definition.id);
  assert.equal(run.path, `.github/workflows/${workflow}`);
  assert.equal(run.repository.id, REPOSITORY_ID);
  assert.equal(run.head_repository.id, REPOSITORY_ID);
  assert.equal(run.status, 'completed');
  assert.ok((workflow === 'ci.yml' && event === 'push' ? ['success', 'failure'] : ['success']).includes(run.conclusion),
    `${workflow} run did not finish with usable job evidence`);
  const { data: suite } = await github.octokit.checks.getSuite({ ...REPO, check_suite_id: run.check_suite_id });
  assert.equal(suite.app.slug, 'github-actions');
  assert.equal(suite.app.id, 15368);
  const actualJobs = await github.octokit.paginate('GET /repos/{owner}/{repo}/actions/runs/{run_id}/attempts/{attempt_number}/jobs', {
    ...REPO, run_id: run.id, attempt_number: run.run_attempt, per_page: 100,
  });
  for (const name of jobs) {
    const matching = actualJobs.filter(job => job.name === name);
    assert.equal(matching.length, 1, `expected exactly one ${workflow}/${name}`);
    assert.equal(matching[0].run_id, run.id);
    assert.equal(matching[0].head_sha, sha);
    assert.equal(matching[0].status, 'completed');
    assert.equal(matching[0].conclusion, 'success', `${workflow}/${name} must succeed`);
  }
  return { id: run.id, attempt: run.run_attempt, workflowId: definition.id, headSha: sha };
}

export async function readinessArtifact(github, run) {
  const artifacts = await github.octokit.paginate(github.octokit.actions.listWorkflowRunArtifacts, {
    ...REPO, run_id: run.id, per_page: 100,
  });
  const selected = artifacts.filter(artifact => artifact.name === `library-readiness-${run.id}-${run.attempt}`);
  assert.equal(selected.length, 1, 'missing or ambiguous readiness artifact for run attempt');
  const artifact = selected[0];
  assert.equal(artifact.expired, false, 'readiness artifact expired');
  assert.equal(artifact.workflow_run.id, run.id);
  assert.equal(artifact.workflow_run.head_repository_id, REPOSITORY_ID);
  assert.equal(artifact.workflow_run.repository_id, REPOSITORY_ID);
  assert.equal(artifact.workflow_run.head_sha, run.headSha);
  assert.ok(Number.isSafeInteger(artifact.id) && artifact.id > 0);
  assert.ok(artifact.size_in_bytes > 0 && artifact.size_in_bytes <= 1024 * 1024);
  return artifact.id;
}

export async function downloadArtifact(id, token, fetchImpl = globalThis.fetch) {
  assert.ok(Number.isSafeInteger(id) && id > 0);
  const response = await fetchImpl(`https://api.github.com/repos/grafana/ai-sdk/actions/artifacts/${id}/zip`, {
    headers: { authorization: `Bearer ${token}`, accept: 'application/vnd.github+json', connection: 'close' },
    redirect: 'manual', signal: AbortSignal.timeout(30_000),
  });
  assert.equal(response.status, 302, 'expected authenticated artifact download redirect');
  const destination = new URL(response.headers.get('location'));
  assert.equal(destination.protocol, 'https:');
  assert.equal(destination.username, '');
  assert.equal(destination.password, '');
  assert.equal(destination.port, '');
  assert.ok(destination.hostname.endsWith('.blob.core.windows.net') ||
    destination.hostname.endsWith('.actions.githubusercontent.com'), 'unexpected artifact storage host');
  const download = await fetchImpl(destination.href, {
    headers: { connection: 'close' }, redirect: 'error', signal: AbortSignal.timeout(30_000),
  });
  assert.equal(download.status, 200, 'artifact download failed');
  const chunks = [];
  let size = 0;
  for await (const chunk of download.body) {
    size += chunk.length;
    assert.ok(size <= 1024 * 1024, 'artifact exceeds download limit');
    chunks.push(Buffer.from(chunk));
  }
  return Buffer.concat(chunks);
}

export async function assertUnpublished(github, tag) {
  for (const read of [
    () => github.octokit.git.getRef({ ...REPO, ref: `tags/${tag}` }),
    () => github.octokit.repos.getReleaseByTag({ ...REPO, tag }),
  ]) {
    try { await read(); } catch (error) {
      if (error.status === 404) continue;
      throw error;
    }
    throw new Error(`tag or release already exists: ${tag}; recovery is outside this slice`);
  }
}

export function sameCandidate(before, after) {
  for (const field of ['number', 'title', 'body', 'state', 'merged', 'draft', 'merge_commit_sha', 'user']) {
    assert.deepEqual(after[field], before[field], `candidate changed: ${field}`);
  }
  for (const side of ['head', 'base']) {
    assert.equal(after[side].ref, before[side].ref, `candidate ${side} branch changed`);
    for (const field of ['id', 'full_name']) assert.equal(after[side].repo[field], before[side].repo[field],
      `candidate ${side} repository changed: ${field}`);
  }
  assert.equal(after.head.sha, before.head.sha, 'release head changed');
}
