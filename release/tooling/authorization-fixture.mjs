import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { setLogger } from 'release-please';
import { SOURCE_JOBS } from './authorization-evidence.mjs';
import { REPOSITORY_ID, BOT_ID } from './merged-candidates.mjs';

export const sha = letter => letter.repeat(40);
export const base = sha('a'), headSha = sha('b'), mergeSha = sha('c'), treeSha = sha('d');
export const config = JSON.parse(readFileSync(new URL('../../release-please-config.json', import.meta.url)));
export const notes = '## 0.1.0-alpha.2 (2026-10-22)\n\n* Fix';
export const title = 'chore(main): release 0.1.0-alpha.2';
export const body = `Header\n---\n\n${notes}\n\n---\nFooter`;
export const branch = 'release-please--branches--main';
setLogger(Object.fromEntries(['debug', 'info', 'warn', 'error', 'trace'].map(key => [key, () => {}])));

export function fixture() {
  const repo = { id: REPOSITORY_ID, full_name: 'grafana/ai-sdk' };
  const pr = { number: 10, title, body, state: 'closed', merged: true, draft: false,
    merge_commit_sha: mergeSha, labels: [{ name: 'autorelease: pending' }], user: { id: BOT_ID, login: 'grafana-ai-sdk-release[bot]', type: 'Bot' },
    head: { sha: headSha, ref: branch, repo }, base: { sha: mergeSha, ref: 'main', repo } };
  const candidate = { repositoryId: REPOSITORY_ID, component: '.', version: '0.1.0-alpha.2',
    tag: 'v0.1.0-alpha.2', notes, name: 'v0.1.0-alpha.2', prNumber: 10, headSha, mergeSha,
    parents: [base], treeSha, config, pr, draft: false, prerelease: true };
  const head = { sha: headSha, author: { id: BOT_ID }, commit: { verification: { verified: true, reason: 'valid' } } };
  const missing = async () => { throw Object.assign(new Error('missing'), { status: 404 }); };
  const github = { repository: { owner: 'grafana', repo: 'ai-sdk', defaultBranch: 'main' },
    getFileJson: async path => path === 'release-please-config.json' ? config : { '.': '0.1.0-alpha.1' },
    async *pullRequestIterator(target, state, max, files) {
      assert.equal(target, 'main'); assert.equal(state, 'MERGED');
      assert.equal(max, Number.MAX_SAFE_INTEGER); assert.equal(files, false);
      for (let i = 0; i < 200; i++) yield { number: i + 100, labels: [], title: 'fix: ordinary', body: '', sha: base };
      yield { number: 10, title, body, sha: mergeSha, labels: ['autorelease: pending'],
        headBranchName: branch, baseBranchName: 'main', files: [] };
    },
    octokit: {
      repos: { getCommit: async ({ ref }) => ({ data: ref === headSha ? head :
        { sha: ref === 'main' ? base : mergeSha, parents: [{ sha: base }], commit: { tree: { sha: treeSha } } } }),
      getReleaseByTag: missing, compareCommits: async () => ({ data: { status: 'ahead' } }) },
      pulls: { get: async () => ({ data: structuredClone(pr) }) },
      git: { getRef: missing },
      actions: { getWorkflow: async ({ workflow_id }) => ({ data: { id: workflow_id === 'ci.yml' ? 100 : 200 } }),
        listWorkflowRuns() {}, listWorkflowRunArtifacts() {} },
      checks: { getSuite: async () => ({ data: { app: { slug: 'github-actions', id: 15368 } } }) },
      paginate: async (route, params) => {
        if (route === github.octokit.actions.listWorkflowRuns) {
          return [{ id: params.workflow_id, run_attempt: 1, workflow_id: params.workflow_id,
            path: `.github/workflows/${params.workflow_id === 100 ? 'ci.yml' : 'release-readiness.yml'}`,
            event: params.event, head_sha: params.head_sha, head_branch: params.event === 'push' ? 'main' : branch,
            repository: repo, head_repository: repo, status: 'completed', conclusion: 'success', check_suite_id: 99 }];
        }
        if (route === github.octokit.actions.listWorkflowRunArtifacts) {
          return [{ id: 77, name: 'library-readiness-200-1', expired: false, size_in_bytes: 500,
            workflow_run: { id: 200, head_sha: headSha, repository_id: REPOSITORY_ID, head_repository_id: REPOSITORY_ID } }];
        }
        assert.match(route, /attempts/);
        return (params.run_id === 100 ? SOURCE_JOBS : ['release-readiness']).map(name => ({ name,
          run_id: params.run_id, head_sha: params.run_id === 100 ? mergeSha : headSha,
          status: 'completed', conclusion: 'success' }));
      },
    },
  };
  return { github, candidate, head, inventory: { mainSha: base, candidates: [candidate] } };
}

