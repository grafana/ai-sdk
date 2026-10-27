import assert from 'node:assert/strict';
import { appendFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import { GitHub } from 'release-please';
import { readOnlyFetch } from './preview.mjs';
import { ROOT_BRANCH } from './root-pr-exercise.mjs';

const repo = { owner: 'grafana', repo: 'ai-sdk' };

export function verifyIdentity(pr, commit, base) {
  assert.equal(pr.state, 'open');
  assert.equal(pr.draft, false);
  assert.equal(pr.user.id, 332654929);
  assert.equal(pr.user.login, 'grafana-ai-sdk-release[bot]');
  assert.equal(pr.user.type, 'Bot');
  for (const side of ['head', 'base']) {
    assert.equal(pr[side].repo.id, 1315458055);
    assert.equal(pr[side].repo.full_name, 'grafana/ai-sdk');
  }
  assert.equal(pr.head.ref, ROOT_BRANCH);
  assert.equal(pr.base.ref, 'main');
  assert.equal(commit.sha, pr.head.sha);
  assert.equal(commit.author.id, 332654929);
  assert.equal(commit.commit.verification.verified, true);
  assert.equal(commit.commit.verification.reason, 'valid');
  assert.deepEqual(commit.parents.map(parent => parent.sha), [base]);
  assert.deepEqual(commit.files.map(file => file.filename).sort(), ['.release-please-manifest.json', 'CHANGELOG.md']);
}

export async function verifyChecks(github, pr) {
  for (const [workflow, jobName] of [['ci.yml', 'ci'], ['release-readiness.yml', 'release-readiness']]) {
    const { data } = await github.octokit.actions.listWorkflowRuns({
      ...repo, workflow_id: workflow, event: 'pull_request', head_sha: pr.head.sha, per_page: 100,
    });
    const run = data.workflow_runs.filter(run => run.event === 'pull_request' &&
      run.head_sha === pr.head.sha && run.head_branch === ROOT_BRANCH &&
      run.head_repository?.id === 1315458055 &&
      run.pull_requests.some(item => item.number === pr.number))
      .sort((a, b) => b.id - a.id)[0];
    if (!run) return false;
    assert.equal(run.actor?.id, 332654929, `${workflow} must be triggered by the release App`);
    if (run.status !== 'completed') return false;
    assert.equal(run.conclusion, 'success', `${workflow} did not succeed`);
    const jobs = await github.octokit.paginate(github.octokit.actions.listJobsForWorkflowRun, {
      ...repo, run_id: run.id, filter: 'latest', per_page: 100,
    });
    const job = jobs.find(job => job.name === jobName);
    assert.equal(job?.conclusion, 'success', `${workflow}/${jobName} must run and succeed`);
  }
  return true;
}

async function main() {
  const number = Number(process.argv[2]);
  assert.ok(Number.isSafeInteger(number) && number > 0, 'provide the test PR number');
  assert.match(process.env.EXPECTED_BASE_SHA ?? '', /^[a-f0-9]{40}$/);
  assert.ok(process.env.GH_TOKEN);
  const github = await GitHub.create({ ...repo, defaultBranch: 'main', token: process.env.GH_TOKEN,
    fetch: readOnlyFetch() });
  let expectedHead, expectedTitle, expectedBody;
  for (let attempt = 0; attempt < 120; attempt++) {
    const { data: pr } = await github.octokit.pulls.get({ ...repo, pull_number: number });
    expectedHead ??= pr.head.sha;
    expectedTitle ??= pr.title;
    expectedBody ??= pr.body;
    assert.equal(pr.title, expectedTitle, 'test PR title changed while awaiting CI');
    assert.equal(pr.body, expectedBody, 'test PR body changed while awaiting CI');
    assert.equal(pr.head.sha, expectedHead, 'test PR changed while awaiting CI');
    const { data: commit } = await github.octokit.repos.getCommit({ ...repo, ref: expectedHead });
    verifyIdentity(pr, commit, process.env.EXPECTED_BASE_SHA);
    if (await verifyChecks(github, pr)) {
      const { data: main } = await github.octokit.repos.getCommit({ ...repo, ref: 'main' });
      assert.equal(main.sha, process.env.EXPECTED_BASE_SHA, 'main advanced; the exercise needs current-base evidence');
      const { data: current } = await github.octokit.pulls.get({ ...repo, pull_number: number });
      assert.equal(current.head.sha, expectedHead);
      assert.equal(current.state, 'open');
      assert.equal(current.title, expectedTitle);
      assert.equal(current.body, expectedBody);
      const summary = `App PR #${number}: verified bot identity and signed commit ${expectedHead}; source CI and release readiness succeeded. No publication was performed.\n`;
      console.log(summary);
      if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY, summary);
      return;
    }
    console.log(`Waiting for source CI and release readiness on PR #${number} (${expectedHead}).`);
    await new Promise(resolve => setTimeout(resolve, 15_000));
  }
  throw new Error('timed out waiting for PR-triggered CI and release readiness');
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch(error => { console.error(error.message); process.exitCode = 1; });
}
