import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';

const require = createRequire(new URL('../../../../test/conformance/tools/package.json', import.meta.url));
assert.equal(require('@ai-sdk/openai/package.json').version, '4.0.72');
const { createOpenAI } = await import(pathToFileURL(require.resolve('@ai-sdk/openai')));

const response = {
  id: 'resp_test',
  created_at: 1700000000,
  model: 'gpt-4o',
  object: 'response',
  status: 'completed',
  output: [],
  usage: {
    input_tokens: 1,
    output_tokens: 1,
    total_tokens: 2,
    input_tokens_details: { cached_tokens: 0 },
    output_tokens_details: { reasoning_tokens: 0 },
  },
};

for (const [name, alias] of [
  ['shell', 'terminal'],
  ['local_shell', 'localTerminal'],
  ['tool_search', 'discover'],
]) {
  for (const [selection, toolName] of [['canonical', name], ['alias', alias]]) {
    for (const mode of ['generate', 'stream']) {
      const requests = [];
      const model = createOpenAI({
        apiKey: 'test-key',
        fetch: async (_url, init) => {
          const body = JSON.parse(init.body);
          requests.push({ tools: body.tools, tool_choice: body.tool_choice, ...(body.stream !== undefined && { stream: body.stream }) });
          return mode === 'generate'
            ? Response.json(response)
            : new Response(`event: response.completed\ndata: ${JSON.stringify({ type: 'response.completed', sequence_number: 0, response })}\n\n`, {
                headers: { 'Content-Type': 'text/event-stream' },
              });
        },
      }).responses('gpt-4o');
      const options = {
        prompt: [{ role: 'user', content: [{ type: 'text', text: 'hi' }] }],
        tools: [{ type: 'provider', id: `openai.${name}`, name: alias, args: {} }],
        toolChoice: { type: 'tool', toolName },
      };
      if (mode === 'generate') {
        await model.doGenerate(options);
      } else {
        const result = await model.doStream(options);
        for await (const part of result.stream) {
          assert.notEqual(part.type, 'error', JSON.stringify(part));
        }
      }
      assert.equal(requests.length, 1);
      await writeFile(new URL(`${name}-${selection}-${mode}.json`, import.meta.url), JSON.stringify(requests[0], null, 2) + '\n');
    }
  }
}
