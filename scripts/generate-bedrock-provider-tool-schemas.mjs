import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const baseline = readFileSync(join(root, 'test/conformance/upstream.yaml'), 'utf8');
const pins = [
  'commit: 08ae5ad05bc12496dd1ffcf64e34419e0831300d',
  '"@ai-sdk/anthropic": 4.0.58',
  '"@ai-sdk/provider-utils": 5.0.45',
  '"@ai-sdk/provider": 4.0.17',
];
if (pins.some(pin => !baseline.split('\n').some(line => line.trim() === pin))) {
  throw new Error('upstream baseline changed; update the generator and schemas together');
}

const unsupported = new Set([
  'anthropic.web_search_20250305',
  'anthropic.web_search_20260318',
  'anthropic.web_fetch_20260318',
]);
const target = join(root, 'providers/bedrock/provider_tool_schemas.go');
const workdir = mkdtempSync(join(tmpdir(), 'bedrock-provider-tool-schemas-'));

try {
  execFileSync('npm', [
    'install', '--prefix', workdir, '--no-save', '--ignore-scripts', '--no-audit', '--no-fund',
    '--package-lock=false', '@ai-sdk/anthropic@4.0.58', '@ai-sdk/provider-utils@5.0.45',
    '@ai-sdk/provider@4.0.17', 'zod@3.25.76',
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
  const schemas = JSON.parse(execFileSync(process.execPath, [extract], { encoding: 'utf8' }));
  for (const id of unsupported) {
    if (!(id in schemas)) {
      throw new Error(`filtered tool missing from pinned catalog: ${id}`);
    }
    delete schemas[id];
  }

  const catalog = readFileSync(join(root, 'providers/bedrock/provider_tool_catalog.go'), 'utf8');
  const supported = new Set([...catalog.matchAll(/^\s*"(anthropic\.[^"]+)":/gm)].map(match => match[1]));
  if (supported.size !== Object.keys(schemas).length || Object.keys(schemas).some(id => !supported.has(id))) {
    throw new Error('pinned schema IDs do not match the supported provider-tool catalog');
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
  if (process.argv.includes('--check')) {
    if (readFileSync(target, 'utf8') !== output) {
      throw new Error(`${target} differs from the pinned upstream schemas`);
    }
  } else {
    writeFileSync(target, output);
  }
  console.log(`${Object.keys(schemas).length} supported Anthropic provider-tool schemas ${process.argv.includes('--check') ? 'verified' : 'written'}`);
} finally {
  rmSync(workdir, { recursive: true, force: true });
}
