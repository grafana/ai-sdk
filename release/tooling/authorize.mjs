import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { GitHub, setLogger } from 'release-please';
import { readOnlyFetch } from './preview.mjs';
import { discoverMerged, duplicateCandidates, REPO, SHA } from './merged-candidates.mjs';
import { verifyMergedIdentity, successfulRun, readinessArtifact, downloadArtifact,
  assertUnpublished, sameCandidate, SOURCE_JOBS } from './authorization-evidence.mjs';

const CANONICAL = 'https://github.com/grafana/ai-sdk.git';
const tooling = resolve(fileURLToPath(new URL('../..', import.meta.url)));

export function childEnvironment(env = process.env) {
  return { ...Object.fromEntries(['PATH', 'HOME', 'TMPDIR', 'SYSTEMROOT'].filter(key => env[key]).map(key => [key, env[key]])),
    PYTHONDONTWRITEBYTECODE: '1', GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: '/dev/null',
    GIT_TERMINAL_PROMPT: '0', GOWORK: 'off', GOENV: 'off' };
}

function git(args, cwd = tooling) {
  return execFileSync('git', args, { cwd, env: childEnvironment(), encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

export async function authorizeInventory(github, inventory, validate, toolingRevision) {
  const report = { schemaVersion: 1, publicationEnabled: false, mainSha: inventory.mainSha,
    toolingRevision, candidates: [] };
  const duplicates = duplicateCandidates(inventory.candidates);
  for (const candidate of inventory.candidates) {
    const identity = { repositoryId: candidate.repositoryId, component: candidate.component,
      version: candidate.version, tag: candidate.tag, prNumber: candidate.prNumber,
      headSha: candidate.headSha, mergeSha: candidate.mergeSha, parents: candidate.parents,
      treeSha: candidate.treeSha, name: candidate.name, notes: candidate.notes };
    try {
      assert.equal(duplicates.has(candidate.prNumber), false, 'ambiguous component or tag in frozen inventory');
      assert.notEqual(candidate.component, 'ai-gateway', 'Gateway is unsupported by library authorization');
      await assertUnpublished(github, candidate.tag);
      const { data: head } = await github.octokit.repos.getCommit({ ...REPO, ref: candidate.headSha });
      verifyMergedIdentity(candidate, head);
      const source = await successfulRun(github, 'ci.yml', 'push', candidate.mergeSha, 'main', SOURCE_JOBS);
      const readiness = await successfulRun(github, 'release-readiness.yml', 'pull_request',
        candidate.headSha, candidate.pr.head.ref, ['release-readiness']);
      const artifactId = await readinessArtifact(github, readiness);
      const library = await validate(candidate, readiness, artifactId);
      const { data: current } = await github.octokit.pulls.get({ ...REPO, pull_number: candidate.prNumber });
      sameCandidate(candidate.pr, current);
      assert.deepEqual(await successfulRun(github, 'ci.yml', 'push', candidate.mergeSha, 'main', SOURCE_JOBS), source,
        'source evidence changed during validation');
      assert.deepEqual(await successfulRun(github, 'release-readiness.yml', 'pull_request',
        candidate.headSha, candidate.pr.head.ref, ['release-readiness']), readiness, 'readiness evidence changed');
      assert.equal(await readinessArtifact(github, readiness), artifactId, 'readiness artifact changed');
      await assertUnpublished(github, candidate.tag);
      const { data: main } = await github.octokit.repos.getCommit({ ...REPO, ref: 'main' });
      const { data: comparison } = await github.octokit.repos.compareCommits({
        ...REPO, base: candidate.mergeSha, head: main.sha,
      });
      assert.ok(['ahead', 'identical'].includes(comparison.status), 'merge no longer belongs to canonical main');
      report.candidates.push({ ...identity, status: 'approved', source, readiness, artifactId, library });
    } catch (error) {
      report.candidates.push({ ...identity, status: 'blocked', reason: error.message });
    }
  }
  return report;
}

async function main(args) {
  assert.ok(args.length <= 1 && args.every(arg => !arg.startsWith('-')), 'usage: node authorize.mjs [component]');
  assert.ok(process.env.GH_TOKEN, 'GH_TOKEN is required (read-only access is sufficient)');
  assert.equal(git(['status', '--porcelain', '--untracked-files=normal']), '', 'trusted tooling checkout must be clean');
  const toolingRevision = git(['rev-parse', 'HEAD']);
  const logger = Object.fromEntries(['debug', 'info', 'warn', 'error', 'trace'].map(level => [level, (...values) => console.error(...values)]));
  setLogger(logger);
  const github = await GitHub.create({ ...REPO, defaultBranch: 'main', token: process.env.GH_TOKEN,
    logger, fetch: readOnlyFetch() });
  const inventory = await discoverMerged(github, args[0]);
  const validate = async (candidate, run, artifactId) => {
    const temp = mkdtempSync(join(tmpdir(), 'library-authorization-'));
    const checkout = join(temp, 'candidate');
    let added = false;
    try {
      const archive = join(temp, 'evidence.zip');
      writeFileSync(archive, await downloadArtifact(artifactId, process.env.GH_TOKEN));
      const helper = join(tooling, 'scripts/release_authorization.py');
      const record = JSON.parse(execFileSync('python3', [helper, 'record', archive], {
        env: childEnvironment(), encoding: 'utf8', maxBuffer: 1024 * 1024,
      }));
      assert.equal(record.runId, String(run.id));
      assert.equal(record.runAttempt, String(run.attempt));
      for (const sha of [candidate.mergeSha, candidate.headSha, inventory.mainSha]) {
        assert.match(sha, SHA);
        git(['fetch', '--no-tags', CANONICAL, sha]);
      }
      git(['merge-base', '--is-ancestor', candidate.mergeSha, inventory.mainSha]);
      git(['worktree', 'add', '--detach', checkout, candidate.mergeSha]);
      added = true;
      const payload = join(temp, 'input.json');
      const result = join(temp, 'result.json');
      writeFileSync(payload, JSON.stringify({ candidate, pr: candidate.pr, record }));
      try {
        execFileSync('python3', [helper, 'validate', payload, '--result', result], {
          cwd: checkout, env: childEnvironment(), maxBuffer: 8 * 1024 * 1024,
          stdio: ['ignore', 'pipe', 'pipe'],
        });
      } catch (error) {
        throw new Error(`selected-library validation failed: ${error.stderr?.toString().slice(-4000) || error.message}`);
      }
      return JSON.parse(readFileSync(result, 'utf8'));
    } finally {
      if (added) git(['worktree', 'remove', '--force', checkout]);
      rmSync(temp, { recursive: true, force: true });
    }
  };
  const report = await authorizeInventory(github, inventory, validate, toolingRevision);
  console.log(JSON.stringify(report, null, 2));
  if (report.candidates.some(candidate => candidate.status !== 'approved')) process.exitCode = 1;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main(process.argv.slice(2)).catch(error => { console.error(error.message); process.exitCode = 1; });
}
