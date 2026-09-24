import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { readBaseline } from '../test/conformance/tools/upgrade-baseline.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const unsupported = new Set([
  'anthropic.web_search_20250305',
  'anthropic.web_search_20260318',
  'anthropic.web_fetch_20260318',
]);

export function readSchemaBaseline(yaml) {
  const { repository, commit, packages } = readBaseline(yaml);
  if (repository !== 'https://github.com/vercel/ai') {
    throw new Error(`unexpected upstream repository: ${repository}`);
  }
  for (const name of ['@ai-sdk/anthropic', '@ai-sdk/provider', '@ai-sdk/provider-utils']) {
    if (!packages[name]) {
      throw new Error(`registered baseline is missing ${name}`);
    }
  }
  return {
    commit,
    anthropic: packages['@ai-sdk/anthropic'],
    provider: packages['@ai-sdk/provider'],
    providerUtils: packages['@ai-sdk/provider-utils'],
  };
}

export function readAnthropicZodVersion(lockfile) {
  const marker = '  packages/anthropic:\n';
  const start = lockfile.indexOf(marker);
  if (start === -1) {
    throw new Error('pinned upstream lockfile is missing packages/anthropic');
  }
  const rest = lockfile.slice(start + marker.length);
  const nextImporter = rest.search(/^  \S/m);
  const importer = nextImporter === -1 ? rest : rest.slice(0, nextImporter);
  const version = importer.match(/^      zod:\r?\n        specifier: \S+\r?\n        version: (\S+)\r?$/m)?.[1];
  if (!version || !/^\d+\.\d+\.\d+$/.test(version)) {
    throw new Error('pinned upstream Anthropic lockfile has no stable zod version');
  }
  return version;
}

export function selectSupportedSchemas(source, supported) {
  if (supported.size === 0) {
    throw new Error('Go provider-tool catalog is empty');
  }
  const missing = [...supported].filter(id => !Object.hasOwn(source, id)).sort();
  if (missing.length > 0) {
    throw new Error(`Go-supported provider-tool schemas missing upstream: ${missing.join(', ')}`);
  }
  return {
    schemas: Object.fromEntries([...supported].map(id => [id, source[id]])),
    newToolIds: Object.keys(source).filter(id => !supported.has(id) && !unsupported.has(id)).sort(),
  };
}

async function main(args) {
  if (args.length > 1 || (args.length === 1 && args[0] !== '--check')) {
    throw new Error('usage: node scripts/generate-bedrock-provider-tool-schemas.mjs [--check]');
  }
  const check = args[0] === '--check';
  const { commit, anthropic, provider, providerUtils } = readSchemaBaseline(
    readFileSync(join(root, 'test/conformance/upstream.yaml'), 'utf8'),
  );
  const response = await fetch(`https://raw.githubusercontent.com/vercel/ai/${commit}/pnpm-lock.yaml`, {
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) {
    throw new Error(`fetching pinned upstream lockfile failed: HTTP ${response.status}`);
  }
  const zod = readAnthropicZodVersion(await response.text());
  const target = join(root, 'providers/bedrock/provider_tool_schemas.go');
  const workdir = mkdtempSync(join(tmpdir(), 'bedrock-provider-tool-schemas-'));

  try {
    execFileSync('npm', [
      'install', '--prefix', workdir, '--no-save', '--ignore-scripts', '--no-audit', '--no-fund',
      '--package-lock=false', `@ai-sdk/anthropic@${anthropic}`, `@ai-sdk/provider-utils@${providerUtils}`,
      `@ai-sdk/provider@${provider}`, `zod@${zod}`,
    ], { stdio: 'inherit' });

    const extract = join(workdir, 'extract.mjs');
    writeFileSync(extract, `import { anthropicTools } from '@ai-sdk/anthropic/internal';
import { asSchema } from '@ai-sdk/provider-utils';
const schemas = {};
for (const factory of Object.values(anthropicTools)) {
  const tool = factory({});
  schemas[tool.id] = await asSchema(tool.inputSchema).jsonSchema;
}
console.log(JSON.stringify(schemas));
`);
    const source = JSON.parse(execFileSync(process.execPath, [extract], { encoding: 'utf8' }));
    const catalog = readFileSync(join(root, 'providers/bedrock/provider_tool_catalog.go'), 'utf8');
    const supported = new Set([...catalog.matchAll(/^\s*"(anthropic\.[^"]+)":/gm)].map(match => match[1]));
    const { schemas, newToolIds } = selectSupportedSchemas(source, supported);
    if (newToolIds.length > 0) {
      console.warn(`Upstream provider tools not in the Go catalog; assess for parity work: ${newToolIds.join(', ')}`);
    }

    const lines = ['package bedrock', '', 'import "encoding/json"', '', 'var anthropicProviderToolSchemas = map[string]json.RawMessage{'];
    for (const id of Object.keys(schemas).sort()) {
      const schema = JSON.stringify(schemas[id], null, 2);
      if (schema.includes('`')) {
        throw new Error(`schema cannot be embedded as a Go raw string: ${id}`);
      }
      lines.push(`\t${JSON.stringify(id)}: json.RawMessage(\`${schema}\`),`);
    }
    lines.push('}', '');
    const output = execFileSync('gofmt', { input: lines.join('\n'), encoding: 'utf8' });
    if (check) {
      if (readFileSync(target, 'utf8') !== output) {
        throw new Error(`${target} differs from the pinned upstream schemas`);
      }
    } else {
      writeFileSync(target, output);
    }
    console.log(`${Object.keys(schemas).length} supported Anthropic provider-tool schemas ${check ? 'verified' : 'written'}`);
  } finally {
    rmSync(workdir, { recursive: true, force: true });
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    await main(process.argv.slice(2));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
