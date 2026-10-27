import { pathToFileURL } from 'node:url';
import { GitHub, Manifest, setLogger } from 'release-please';
import { LIBRARY_BRANCH, generateRequirements, withRequirements } from './requirements.mjs';

export function readOnlyFetch(fetchImpl = globalThis.fetch) {
  return async (url, options = {}) => {
    const target = new URL(url);
    const method = (options.method ?? 'GET').toUpperCase();
    let allowed = method === 'GET';
    if (method === 'POST' && target.pathname === '/graphql') {
      const { query } = JSON.parse(options.body);
      allowed = typeof query === 'string' &&
        /^\s*query\b/.test(query) && !/\b(mutation|subscription)\b/.test(query);
    }
    if (target.origin !== 'https://api.github.com' || !allowed) {
      throw new Error(`preview refused request: ${method} ${target.origin}${target.pathname}`);
    }
    const timeout = AbortSignal.timeout(30_000);
    const signal = options.signal ? AbortSignal.any([options.signal, timeout]) : timeout;
    const headers = new Headers(options.headers);
    headers.set('connection', 'close');
    return fetchImpl(url, { ...options, headers, redirect: 'error', signal });
  };
}

export async function prepareCandidates(github, { branch = 'main', path, manifestOptions = {} } = {}) {
  const manifest = await Manifest.fromManifest(
    github, branch, 'release-please-config.json',
    '.release-please-manifest.json', manifestOptions, path,
  );
  if (path !== undefined && !Object.hasOwn(manifest.repositoryConfig, path)) {
    throw new Error(`unregistered release component: ${path}`);
  }
  return manifest.buildPullRequests();
}

export async function preview(github, options = {}) {
  const branch = options.branch ?? 'main';
  const ref = { ...github.repository, ref: branch };
  const before = (await github.octokit.repos.getCommit(ref)).data.sha;
  const candidates = await prepareCandidates(github, options);
  const proposals = [];
  for (const candidate of candidates) {
    if (candidate.headRefName === LIBRARY_BRANCH) {
      const config = await github.getFileJson('release-please-config.json', branch);
      await withRequirements(github, candidate, { sha: before, config, generate: options.generate ?? generateRequirements });
    }
    const changes = await github.buildChangeSet(candidate.updates, branch);
    proposals.push({
      title: candidate.title.toString(),
      branch: candidate.headRefName,
      version: candidate.version?.toString(),
      body: candidate.body.toString(),
      changes: [...changes].map(([path, change]) => ({
        path,
        before: change.originalContent ?? '',
        after: change.content,
      })),
    });
  }
  const after = (await github.octokit.repos.getCommit(ref)).data.sha;
  if (before !== after) {
    throw new Error('target branch advanced during preview; rerun against the current branch');
  }
  return { repository: 'grafana/ai-sdk', branch, sha: before, proposals };
}

async function main(args) {
  if (args.length > 1 || args.some(arg => arg.startsWith('-'))) {
    throw new Error('usage: node preview.mjs [registered-component-path]');
  }
  if (!process.env.GH_TOKEN) throw new Error('GH_TOKEN is required');
  const logger = Object.fromEntries(
    ['debug', 'info', 'warn', 'error', 'trace'].map(level => [
      level, (...values) => console.error(...values),
    ]),
  );
  setLogger(logger);
  const github = await GitHub.create({
    owner: 'grafana', repo: 'ai-sdk', defaultBranch: 'main',
    token: process.env.GH_TOKEN, logger, fetch: readOnlyFetch(),
  });
  console.log(JSON.stringify(await preview(github, { path: args[0] }), null, 2));
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main(process.argv.slice(2)).catch(error => {
    console.error(error.message);
    process.exitCode = 1;
  });
}
