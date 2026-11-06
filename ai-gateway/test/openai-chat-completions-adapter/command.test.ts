// Synthetic deterministic integration upstreams, not provider recordings.
import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { mkdtempSync, writeFileSync, readFileSync, rmSync } from 'node:fs';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { test } from 'node:test';
import OpenAI from 'openai';
import { zodResponseFormat } from 'openai/helpers/zod';
import { z } from 'zod';

test('official JavaScript SDK against platform plugin: Responses, compatible and Anthropic', async (t) => {
  const directory = mkdtempSync(join(tmpdir(), 'chat-completions-adapter-js-'));
  const binary = join(directory, 'gateway');
  const hostBinary = join(directory, 'plugin-host');
  try {
    execFileSync('go', ['build', '-race', '-mod=readonly', '-o', binary, './pkg'], {
      cwd: resolve(import.meta.dirname, '../..'),
      env: { ...process.env, GOWORK: 'off' },
      stdio: 'pipe',
    });
    execFileSync('go', ['build', '-race', '-mod=readonly', '-o', hostBinary, './test/platformhost/cmd/plugin-host'], {
      cwd: resolve(import.meta.dirname, '../..'),
      env: { ...process.env, GOWORK: 'off' },
      stdio: 'pipe',
    });
    for (const backend of ['openai', 'openai-compatible', 'anthropic'] as const) {
      await t.test(backend, async () => {
        const requests: Record<string, any>[] = [];
        let failNext = false;
        let emptyArgumentsNext = false;
        let holdNext = false;
        let truncateNext = false;
        let partialNext = false;
        let emptyNext = false;
        let cancelled: (() => void) | undefined;
        const upstream = createServer(async (req, res) => {
          const chunks: Buffer[] = [];
          for await (const chunk of req) chunks.push(Buffer.from(chunk));
          const body = JSON.parse(Buffer.concat(chunks).toString());
          requests.push(body);
          if (req.url === '/fail/v1/responses') {
            assert.equal(body.store, false);
            assert.equal(body.parallel_tool_calls, false);
            res.writeHead(503, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ error: { type: 'server_error', message: 'retry' } }));
            return;
          }
          if (failNext) {
            failNext = false;
            res.writeHead(503, { 'Content-Type': 'application/json' });
            res.end(
              JSON.stringify({ type: 'error', error: { type: 'overloaded_error', message: 'synthetic retryable' } })
            );
            return;
          }
          const isAnthropic = backend === 'anthropic';
          const truncate = truncateNext;
          truncateNext = false;
          const hold = holdNext;
          holdNext = false;
          if (hold) res.once('close', () => cancelled?.());
          const emptyArguments = emptyArgumentsNext;
          emptyArgumentsNext = false;
          if (!isAnthropic) assert.equal(body.store, false);
          assert.equal(
            req.url,
            isAnthropic ? '/v1/messages' : backend === 'openai' ? '/v1/responses' : '/v1/chat/completions'
          );
          assert.equal(
            isAnthropic ? req.headers['x-api-key'] : req.headers.authorization,
            isAnthropic ? 'fake-key' : 'Bearer fake-key'
          );
          if (backend === 'openai-compatible' && body.tools?.[0]?.type === 'custom') {
            const choices = (choice: object) => Array.from({ length: body.n ?? 1 }, (_, index) => ({ ...choice, index }));
            assert.deepEqual(body.tools[0].custom.format, { type: 'grammar', grammar: { syntax: 'regex', definition: '[a-z]+' } });
            assert.deepEqual(body.tool_choice, { type: 'custom', custom: { name: 'uppercase' } });
            if (!body.stream) {
              res.setHeader('Content-Type', 'application/json');
              res.end(JSON.stringify({ id: 'custom-call', model: body.model, choices: choices({ message: { role: 'assistant', content: null, tool_calls: [{ id: 'custom-1', type: 'custom', custom: { name: 'uppercase', input: 'hello' } }] }, finish_reason: 'tool_calls' }) }));
            } else {
              res.setHeader('Content-Type', 'text/event-stream');
              for (const delta of [{ tool_calls: [{ index: 0, id: 'custom-1', type: 'custom', custom: { name: 'uppercase', input: 'hel' } }] }, { tool_calls: [{ index: 0, custom: { input: 'lo' } }] }]) {
                res.write(`data: ${JSON.stringify({ id: 'custom-call', model: body.model, choices: choices({ delta }) })}\n\n`);
              }
              res.end(`data: ${JSON.stringify({ choices: choices({ delta: {}, finish_reason: 'tool_calls' }) })}\n\ndata: [DONE]\n\n`);
            }
            return;
          }
          const tool =
            !!body.tools?.length &&
            !JSON.stringify(body.messages ?? body.input).includes('tool_result') &&
            !JSON.stringify(body.input ?? []).includes('function_call_output') &&
            !(body.messages ?? []).some((m: any) => m.role === 'tool');
          const text = emptyNext ? '' : body.response_format || body.text ? '{"answer":"yes"}' : 'hello';
          emptyNext = false;
          if (backend === 'openai') {
            const item = tool
              ? {
                  id: 'fc_test',
                  type: 'function_call',
                  call_id: 'call_weather',
                  name: 'weather',
                  arguments: '{"city":"Rio"}',
                  status: 'completed',
                }
              : {
                  id: 'msg_test',
                  type: 'message',
                  role: 'assistant',
                  status: 'completed',
                  content: [{ type: 'output_text', text, annotations: [], logprobs: [] }],
                };
            const response = {
              id: 'resp_private',
              object: 'response',
              created_at: 1,
              status: 'completed',
              model: body.model,
              output: [item],
              usage: {
                input_tokens: 3,
                input_tokens_details: { cached_tokens: 1 },
                output_tokens: 2,
                output_tokens_details: { reasoning_tokens: 1 },
                total_tokens: 5,
              },
            };
            if (!body.stream) {
              res.setHeader('Content-Type', 'application/json');
              res.end(JSON.stringify(response));
              return;
            }
            res.setHeader('Content-Type', 'text/event-stream');
            const events: any[] = [
              {
                type: 'response.created',
                response: { id: 'resp_private', model: body.model, created_at: 1, status: 'in_progress', output: [] },
              },
              {
                type: 'response.output_item.added',
                output_index: 0,
                item: tool ? { ...item, arguments: '' } : { ...item, content: [] },
              },
            ];
            if (tool)
              events.push(
                {
                  type: 'response.function_call_arguments.delta',
                  item_id: 'fc_test',
                  output_index: 0,
                  delta: '{"city":"Rio"}',
                },
                {
                  type: 'response.function_call_arguments.done',
                  item_id: 'fc_test',
                  output_index: 0,
                  arguments: '{"city":"Rio"}',
                }
              );
            else
              events.push({
                type: 'response.output_text.delta',
                item_id: 'msg_test',
                output_index: 0,
                content_index: 0,
                delta: text,
                logprobs: [],
              });
            if (!hold)
              events.push(
                { type: 'response.output_item.done', output_index: 0, item },
                { type: 'response.completed', response }
              );
            for (const [sequence_number, event] of events.entries())
              res.write(`event: ${event.type}\ndata: ${JSON.stringify({ ...event, sequence_number })}\n\n`);
            if (!hold) res.end();
            return;
          }
          if (isAnthropic) {
            const content = tool
              ? [
                  {
                    type: 'tool_use',
                    id: 'call_weather',
                    name: 'weather',
                    input: emptyArguments ? {} : { city: 'Rio' },
                  },
                ]
              : [{ type: 'text', text: 'hello' }];
            const message = {
              id: 'msg_private',
              type: 'message',
              role: 'assistant',
              model: body.model,
              content,
              stop_reason: tool ? 'tool_use' : 'end_turn',
              stop_sequence: null,
              usage: { input_tokens: 3, output_tokens: 2 },
            };
            if (!body.stream) {
              res.setHeader('Content-Type', 'application/json');
              res.end(JSON.stringify(message));
              return;
            }
            res.setHeader('Content-Type', 'text/event-stream');
            const events: any[] = [
              {
                type: 'message_start',
                message: { ...message, content: [], stop_reason: null, usage: { input_tokens: 3, output_tokens: 0 } },
              },
            ];
            if (tool) {
              events.push({
                type: 'content_block_start',
                index: 0,
                content_block: { type: 'tool_use', id: 'call_weather', name: 'weather', input: {} },
              });
              if (!emptyArguments)
                events.push({
                  type: 'content_block_delta',
                  index: 0,
                  delta: { type: 'input_json_delta', partial_json: '{"city":"Rio"}' },
                });
            } else {
              events.push(
                { type: 'content_block_start', index: 0, content_block: { type: 'text', text: '' } },
                { type: 'content_block_delta', index: 0, delta: { type: 'text_delta', text: 'hello' } }
              );
            }
            events.push(
              { type: 'content_block_stop', index: 0 },
              {
                type: 'message_delta',
                delta: { stop_reason: message.stop_reason, stop_sequence: null },
                usage: { output_tokens: 2 },
              },
              { type: 'message_stop' }
            );
            for (const event of events) {
              res.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);
              if (truncate && event.type === 'content_block_delta') break;
            }
            res.end();
            return;
          }
          if (body.n === 2) {
            const base = { id: 'multi-native', model: body.model, created: 123 };
            const usage = { prompt_tokens: 3, completion_tokens: 4, total_tokens: 7 };
            const call = (index: number) => ({ id: 'same-call-id', type: 'function', function: { name: 'weather', arguments: JSON.stringify({ city: index ? 'Rio' : 'Lima' }) } });
            const message = (index: number) => tool ? { role: 'assistant', content: null, tool_calls: [call(index)] } : { role: 'assistant', content: index ? 'second' : 'first' };
            const reason = (index: number) => tool ? 'tool_calls' : index ? 'length' : 'stop';
            if (!body.stream) {
              res.setHeader('Content-Type', 'application/json');
              res.end(JSON.stringify({ ...base, choices: [1, 0].map(index => ({ index, message: message(index), finish_reason: reason(index) })), usage }));
              return;
            }
            res.setHeader('Content-Type', 'text/event-stream');
            for (const index of [1, 0]) {
              const delta = tool ? { tool_calls: [{ ...call(index), index: 0 }] } : { content: index ? 'second' : 'first' };
              res.write(`data: ${JSON.stringify({ ...base, choices: [{ index, delta }] })}\n\n`);
            }
            for (const index of [1, 0]) res.write(`data: ${JSON.stringify({ ...base, choices: [{ index, delta: {}, finish_reason: reason(index) }] })}\n\n`);
            res.end(`data: ${JSON.stringify({ ...base, choices: [], usage })}\n\ndata: [DONE]\n\n`);
            return;
          }
          const partial = partialNext;
          partialNext = false;
          const call = {
            id: 'call_weather',
            type: 'function',
            function: { name: 'weather', arguments: partial ? '{"city":' : '{"city":"Rio"}' },
          };
          const message = tool
            ? { role: 'assistant', content: null, tool_calls: [call] }
            : { role: 'assistant', content: text };
          const finish = partial ? 'length' : tool ? 'tool_calls' : 'stop';
          const base = { id: 'private', created: 1, model: body.model };
          if (!body.stream) {
            res.setHeader('Content-Type', 'application/json');
            res.end(
              JSON.stringify({
                ...base,
                object: 'chat.completion',
                choices: [{ index: 0, message, finish_reason: finish }],
                usage: { prompt_tokens: 3, completion_tokens: 2, total_tokens: 5 },
              })
            );
            return;
          }
          res.setHeader('Content-Type', 'text/event-stream');
          const delta = tool ? { tool_calls: [{ ...call, index: 0 }] } : { content: text };
          for (const choice of [
            { index: 0, delta, finish_reason: null },
            { index: 0, delta: {}, finish_reason: finish },
          ])
            res.write(`data: ${JSON.stringify({ ...base, object: 'chat.completion.chunk', choices: [choice] })}\n\n`);
          res.write(
            `data: ${JSON.stringify({ ...base, object: 'chat.completion.chunk', choices: [], usage: { prompt_tokens: 3, completion_tokens: 2, total_tokens: 5 } })}\n\ndata: [DONE]\n\n`
          );
          res.end();
        });
        await new Promise<void>((r) => upstream.listen(0, '127.0.0.1', r));
        const addr = upstream.address();
        assert.ok(addr && typeof addr !== 'string');
        const baseURL = `http://127.0.0.1:${addr.port}${backend === 'anthropic' ? '' : '/v1'}`;
        const config = join(directory, `${backend}.yaml`);
        const fallbackConfig =
          backend === 'anthropic'
            ? '  public/fallback:\n    name: Fallback\n    primary:\n      provider: local\n      model: claude-sonnet-4-20250514\n    fallback:\n      - provider: local\n        model: claude-3-5-haiku-20241022\n'
            : '';
        writeFileSync(
          config,
          `providers:\n  failing:\n    type: openai\n    apiKeyEnv: CHAT_COMPLETIONS_ADAPTER_JS_KEY\n    baseURL: http://127.0.0.1:${addr.port}/fail/v1\n  local:\n    type: ${backend}\n${backend === 'openai-compatible' ? '    providerName: custom-vendor\n    supportsStructuredOutputs: true\n' : ''}    apiKeyEnv: CHAT_COMPLETIONS_ADAPTER_JS_KEY\n    baseURL: ${baseURL}\nmodels:\n  public/chat:\n    name: Chat\n    primary:\n      provider: local\n      model: ${backend === 'anthropic' ? 'claude-sonnet-4-20250514' : 'gpt-4o-mini'}\n    aliases: [chat]\n  public/mixed:\n    name: Mixed\n    primary:\n      provider: failing\n      model: gpt-4.1\n    fallback:\n      - provider: local\n        model: ${backend === 'anthropic' ? 'claude-sonnet-4-20250514' : 'gpt-4o-mini'}\n${fallbackConfig}`
        );
        const env = Object.fromEntries(
          Object.entries(process.env).filter(
            ([key]) => !['GF_PLUGIN_', 'GRAFANA_', 'AGENTO11Y_', 'SIGIL_'].some((prefix) => key.startsWith(prefix))
          )
        );
        const proc = spawn(
          hostBinary,
          [
            '--plugin',
            binary,
            '--topology',
            'self-hosted',
            '--namespace',
            'stacks-1',
            '--service-account-token',
            'glsa_chat_adapter_test',
            '--route-deadline',
            '10s',
          ],
          {
            env: {
              ...env,
              GF_PLUGIN_MODEL_CATALOG: readFileSync(config, 'utf8'),
              GF_PLUGIN_ACCESS_MODE: 'self-hosted',
              CHAT_COMPLETIONS_ADAPTER_JS_KEY: 'fake-key',
            },
            stdio: ['ignore', 'pipe', 'pipe'],
          }
        );
        let stderr = '';
        proc.stderr.on('data', (chunk) => (stderr += chunk));
        const exited = new Promise<number | null>((r) => proc.once('exit', r));
        try {
          const ready = await new Promise<{ url: string; routePrefix: string }>((resolve, reject) => {
            let output = '';
            const timer = setTimeout(() => reject(new Error(`host startup timed out: ${stderr}`)), 30000);
            proc.once('exit', () => {
              clearTimeout(timer);
              reject(new Error(stderr));
            });
            proc.stdout.on('data', (chunk) => {
              output += chunk;
              if (output.includes('\n')) {
                clearTimeout(timer);
                resolve(JSON.parse(output.split('\n')[0]));
              }
            });
          });
          const client = new OpenAI({
            apiKey: 'glsa_chat_adapter_test',
            baseURL: `${ready.url}${ready.routePrefix}/v1`,
            maxRetries: 0,
          });
          const messages: OpenAI.Chat.Completions.ChatCompletionMessageParam[] = [{ role: 'user', content: 'hello' }];
          const unary = await client.chat.completions.create({ model: 'chat', messages });
          assert.equal(unary.model, backend === 'anthropic' ? 'claude-sonnet-4-20250514' : 'gpt-4o-mini');
          assert.equal(
            unary.id,
            backend === 'anthropic' ? 'msg_private' : backend === 'openai' ? 'resp_private' : 'private'
          );
          assert.equal(unary.choices[0].message.content, 'hello');
          assert.equal(unary.usage?.total_tokens, 5);
          const providerOptions = backend === 'anthropic'
            ? { anthropic: { container: { id: 'sdk-container' } } }
            : backend === 'openai' ? { openai: { user: 'sdk-user' } }
            : { 'custom-vendor': { user: 'sdk-user' } };
          const extended = { model: 'chat', messages, providerOptions };
          await client.chat.completions.create(extended);
          assert.deepEqual(backend === 'anthropic' ? requests.at(-1)!.container : requests.at(-1)!.user,
            backend === 'anthropic' ? { id: 'sdk-container' } : 'sdk-user');
          const extendedStream = await client.chat.completions.create({ ...extended, stream: true });
          for await (const chunk of extendedStream) assert.equal(chunk.object, 'chat.completion.chunk');
          assert.deepEqual(backend === 'anthropic' ? requests.at(-1)!.container : requests.at(-1)!.user,
            backend === 'anthropic' ? { id: 'sdk-container' } : 'sdk-user');
          const history: OpenAI.Chat.Completions.ChatCompletionMessageParam[] = [
            { role: 'developer', content: 'Follow the policy', name: 'policy' },
            { role: 'user', content: 'hello', name: 'alice' },
            { role: 'assistant', content: null, refusal: 'Cannot do that' },
            { role: 'user', content: 'Try this instead' },
          ];
          if (backend === 'openai-compatible') {
            const withHistory = await client.chat.completions.create({ model: 'chat', messages: history });
            assert.equal(withHistory.choices[0].message.content, 'hello');
            const sent = requests.at(-1)!;
            assert.deepEqual(sent.messages, history.map((m) => m.role === 'assistant' ? { ...m, content: '' } : m));
          } else {
            for (const stream of [false, true] as const) {
              const before = requests.length;
              await assert.rejects(() => client.chat.completions.create({ model: 'chat', messages: history, stream }),
                (e: unknown) => e instanceof OpenAI.APIError && e.status === 400);
              assert.equal(requests.length, before);
            }
          }
          if (backend !== 'openai-compatible') {
            const unsupported = [{ seed: 7 }, { presence_penalty: 0 }, { frequency_penalty: 0 }, ...(backend === 'openai' ? [{ stop: 'END' }] : [{ temperature: 0.5, top_p: 0.5 }])];
            for (const fields of unsupported) {
              for (const stream of [false, true] as const) {
                const before = requests.length;
                await assert.rejects(() => client.chat.completions.create({ model: 'chat', messages, ...fields, stream }), (e: unknown) => e instanceof OpenAI.APIError && e.status === 400);
                assert.equal(requests.length, before);
              }
            }
          }
          if (backend === 'openai-compatible') {
            const customTools: OpenAI.Chat.Completions.ChatCompletionTool[] = [{ type: 'custom', custom: { name: 'uppercase', format: { type: 'grammar', grammar: { syntax: 'regex', definition: '[a-z]+' } } } }];
            const toolChoice = { type: 'custom' as const, custom: { name: 'uppercase' } };
            for (const n of [1,2]) {
            const custom = await client.chat.completions.create({ model: 'chat', messages, n, tools: customTools, tool_choice: toolChoice });
            assert.equal(custom.choices.length,n);
            for (const choice of custom.choices) {
              const alternative = choice.message.tool_calls?.[0];
              assert.ok(alternative?.type === "custom");
              assert.equal(alternative.custom.input,"hello");
            }
            const call = custom.choices[0].message.tool_calls?.[0];
            assert.ok(call?.type === 'custom');
            assert.equal(call.custom.input, 'hello');
            const continuation = await client.chat.completions.create({ model: 'chat', messages: [...messages, custom.choices[0].message, { role: 'tool', tool_call_id: call.id, content: call.custom.input.toUpperCase() }] });
            assert.equal(continuation.choices[0].message.content, 'hello');
            assert.equal(requests.at(-1)?.messages[1].tool_calls[0].custom.input, 'hello');
            const stream = await client.chat.completions.create({ model: 'chat', messages, n, tools: customTools, tool_choice: toolChoice, stream: true });
            const inputs = new Map<number,string>();
            for await (const chunk of stream) {
              for (const choice of chunk.choices) {
                for (const delta of choice.delta.tool_calls ?? []) {
                  assert.equal(delta.index,0);
                  inputs.set(choice.index, (inputs.get(choice.index) ?? '') + ((delta as unknown as { custom?: { input?: string } }).custom?.input ?? ''));
                }
              }
            }
            assert.equal(inputs.size,n);
            for (const input of inputs.values()) assert.equal(input, 'hello');
            }
          }
          const tools: OpenAI.Chat.Completions.ChatCompletionTool[] = [
            {
              type: 'function',
              function: {
                name: 'weather',
                parameters: { type: 'object', properties: { city: { type: 'string' } }, required: ['city'] },
              },
            },
          ];
          const legacyFunctions = [{ name: 'weather', parameters: { type: 'object', properties: { city: { type: 'string' } } } }];
          const legacyRequest = { model: 'public/mixed', messages, functions: legacyFunctions, function_call: { name: 'weather' } };
          const legacy = await client.chat.completions.create(legacyRequest);
          assert.equal(legacy.choices[0].finish_reason, 'function_call');
          assert.equal(legacy.choices[0].message.function_call?.name, 'weather');
          assert.deepEqual(JSON.parse(legacy.choices[0].message.function_call!.arguments), { city: 'Rio' });
          assert.equal(legacy.choices[0].message.tool_calls, undefined);
          const legacyHistory: OpenAI.Chat.Completions.ChatCompletionMessageParam[] = [
            ...messages, legacy.choices[0].message, { role: 'function', name: 'weather', content: 'sunny' },
          ];
          const legacyNext = await client.chat.completions.create({ model: 'chat', messages: legacyHistory, functions: legacyFunctions });
          assert.equal(legacyNext.choices[0].message.content, 'hello');
          const legacyStream = await client.chat.completions.create({ ...legacyRequest, stream: true });
          let legacyArguments = '', legacyName = '', legacyFinish = '';
          for await (const chunk of legacyStream) {
            const choice = chunk.choices[0];
            if (!choice) continue;
            assert.equal(choice.delta.tool_calls, undefined);
            legacyName += choice.delta.function_call?.name ?? '';
            legacyArguments += choice.delta.function_call?.arguments ?? '';
            legacyFinish = choice.finish_reason ?? legacyFinish;
          }
          assert.equal(legacyName, 'weather');
          assert.deepEqual(JSON.parse(legacyArguments), { city: 'Rio' });
          assert.equal(legacyFinish, 'function_call');
          if (backend === 'openai-compatible') {
            for (const mode of ["text", "tools", "legacy"] as const) {
              const withTools = mode !== "text";
              const legacy = mode === "legacy";
              const multi = { model: 'chat', messages, n: 2, ...(legacy ? { functions: tools.flatMap(tool => tool.type === "function" ? [tool.function] : []) } : withTools ? { tools } : {}) };
              let before = requests.length;
              const result = await client.chat.completions.create(multi);
              assert.equal(requests.length, before + 1);
              assert.equal(result.usage?.total_tokens, 7);
              assert.deepEqual(result.choices.map(c => c.index), [1, 0]);
              for (const choice of result.choices) {
                if (withTools) {
                  const call = choice.message.tool_calls?.[0];
                  const fn = legacy ? choice.message.function_call : call?.type === "function" ? call.function : undefined;
                  assert.ok(fn);
                  assert.deepEqual(JSON.parse(fn.arguments), { city: choice.index ? 'Rio' : 'Lima' });
                } else assert.equal(choice.message.content, choice.index ? 'second' : 'first');
              }
              before = requests.length;
              const stream = await client.chat.completions.create({ ...multi, stream: true, stream_options: { include_usage: true } });
              const assembled = new Map<number, string>(); const reasons = new Map<number, string>(); let total = 0, usageChunks = 0;
              for await (const chunk of stream) {
                if (chunk.usage) { total += chunk.usage.total_tokens; usageChunks++; }
                for (const choice of chunk.choices) {
                  const delta = legacy ? choice.delta.function_call?.arguments : withTools ? choice.delta.tool_calls?.[0]?.function?.arguments : choice.delta.content;
                  assembled.set(choice.index, (assembled.get(choice.index) ?? '') + (delta ?? ''));
                  if (choice.finish_reason) reasons.set(choice.index, choice.finish_reason);
                }
              }
              assert.equal(requests.length, before + 1); assert.equal(total, 7); assert.equal(usageChunks, 1);
              assert.equal(assembled.get(0), withTools ? '{"city":"Lima"}' : 'first');
              assert.equal(assembled.get(1), withTools ? '{"city":"Rio"}' : 'second');
              assert.equal(reasons.get(0), legacy ? 'function_call' : withTools ? 'tool_calls' : 'stop'); assert.equal(reasons.get(1), legacy ? 'function_call' : withTools ? 'tool_calls' : 'length');
            }
          } else {
            const before = requests.length;
            await assert.rejects(() => client.chat.completions.create({ model: 'chat', messages, n: 2 }), (e: unknown) => e instanceof OpenAI.APIError && e.status === 400);
            assert.equal(requests.length, before);
          }
          for (const streaming of [false, true] as const) {
            const before = requests.length;
            if (streaming) {
              const mixed = await client.chat.completions.create({
                model: 'public/mixed',
                messages,
                tools,
                parallel_tool_calls: false,
                stream: true,
              });
              let finished = false;
              for await (const chunk of mixed) {
                assert.equal(
                  chunk.id,
                  backend === 'anthropic' ? 'msg_private' : backend === 'openai' ? 'resp_private' : 'private'
                );
                if (chunk.choices[0]?.finish_reason === 'tool_calls') finished = true;
              }
              assert.ok(finished);
            } else {
              const mixed = await client.chat.completions.create({
                model: 'public/mixed',
                messages,
                tools,
                parallel_tool_calls: false,
              });
              assert.equal(mixed.choices[0].finish_reason, 'tool_calls');
            }
            assert.equal(requests.length, before + 2);
            const native = requests.at(-1)!;
            if (backend === 'anthropic') assert.equal(native.tool_choice.disable_parallel_tool_use, true);
            else {
              assert.equal(native.parallel_tool_calls, false);
              assert.equal(native.store, false);
              assert.ok(!('parallelToolCalls' in native));
            }
          }
          if (backend === 'openai-compatible') {
            partialNext = true;
            const partial = await client.chat.completions.create({ model: 'chat', messages, tools });
            assert.equal(partial.choices[0].finish_reason, 'length');
            const partialCall = partial.choices[0].message.tool_calls?.[0];
            assert.ok(partialCall?.type === 'function');
            assert.equal(partialCall.function.arguments, '{"city":');
            partialNext = true;
            const partialStream = await client.chat.completions.create({ model: 'chat', messages, tools, stream: true });
            let argumentsText = '', terminal = '';
            for await (const chunk of partialStream) {
              for (const choice of chunk.choices) {
                argumentsText += choice.delta.tool_calls?.[0]?.function?.arguments ?? '';
                terminal = choice.finish_reason ?? terminal;
              }
            }
            assert.equal(argumentsText, '{"city":');
            assert.equal(terminal, 'length');
            emptyNext = true;
            const empty = await client.chat.completions.create({ model: 'chat', messages });
            assert.equal(empty.choices[0].message.content, '');
            assert.equal(empty.choices[0].finish_reason, 'stop');
          }
          const called = await client.chat.completions.create({ model: 'chat', messages, tools });
          assert.equal(called.choices[0].finish_reason, 'tool_calls');
          assert.equal(called.choices[0].message.tool_calls?.[0].id, 'call_weather');
          const continued = await client.chat.completions.create({
            model: 'chat',
            messages: [
              ...messages,
              called.choices[0].message,
              { role: 'tool', tool_call_id: 'call_weather', content: 'sunny' },
            ],
            tools,
          });
          assert.equal(continued.choices[0].message.content, 'hello');
          for (const withTools of [false, true]) {
            const stream = await client.chat.completions.create({
              model: 'chat',
              messages,
              ...(withTools ? { tools } : {}),
              stream: true,
              stream_options: { include_usage: true },
            });
            let text = '',
              args = '',
              finishes = 0,
              usage = 0;
            for await (const chunk of stream) {
              if (!chunk.choices.length) usage = chunk.usage?.total_tokens ?? 0;
              for (const choice of chunk.choices) {
                text += choice.delta.content ?? '';
                for (const call of choice.delta.tool_calls ?? []) args += call.function?.arguments ?? '';
                if (choice.finish_reason) finishes++;
              }
            }
            assert.equal(finishes, 1);
            assert.equal(usage, 5);
            if (withTools) assert.deepEqual(JSON.parse(args), { city: 'Rio' });
            else assert.equal(text, 'hello');
          }
          const before = requests.length;
          await assert.rejects(
            () => client.chat.completions.create({ model: 'chat', messages, n: 129 }),
            (error) => error instanceof OpenAI.APIError && error.status === 400
          );
          assert.equal(requests.length, before);
          if (backend === 'anthropic') {
            emptyArgumentsNext = true;
            const emptyStream = await client.chat.completions.create({
              model: 'chat',
              messages,
              tools: [
                { type: 'function', function: { name: 'weather', parameters: { type: 'object', properties: {} } } },
              ],
              stream: true,
            });
            const fragments: string[] = [];
            let emptyFinishes = 0;
            for await (const chunk of emptyStream) {
              for (const choice of chunk.choices) {
                for (const call of choice.delta.tool_calls ?? []) {
                  if (call.function?.arguments) fragments.push(call.function.arguments);
                }
                if (choice.finish_reason) {
                  assert.equal(choice.finish_reason, 'tool_calls');
                  emptyFinishes++;
                }
              }
            }
            assert.deepEqual(fragments, ['{}']);
            assert.equal(emptyFinishes, 1);
            failNext = true;
            const count = requests.length;
            const recovered = await client.chat.completions.create({ model: 'public/fallback', messages });
            assert.equal(recovered.model, 'claude-3-5-haiku-20241022');
            assert.equal(recovered.choices[0].message.content, 'hello');
            assert.equal(requests.length, count + 2);
            const toolFallback = await client.chat.completions.create({ model: 'public/fallback', messages, tools });
            assert.equal(toolFallback.choices[0].finish_reason, 'tool_calls');
          }
          if (backend !== 'anthropic') {
            const completion = await client.chat.completions.create({
              model: 'chat',
              messages,
              response_format: {
                type: 'json_schema',
                json_schema: {
                  name: 'answer',
                  strict: true,
                  schema: {
                    type: 'object',
                    properties: { answer: { type: 'string' } },
                    required: ['answer'],
                    additionalProperties: false,
                  },
                },
              },
            });
            assert.deepEqual(JSON.parse(completion.choices[0].message.content!), { answer: 'yes' });
            const parsed = await client.chat.completions.parse({
              model: 'chat', messages,
              response_format: zodResponseFormat(z.object({ answer: z.union([z.string(), z.null()]) }), 'answer'),
            });
            assert.deepEqual(parsed.choices[0].message.parsed, { answer: 'yes' });
            holdNext = true;
            const upstreamCancelled = new Promise<void>((resolve) => (cancelled = resolve));
            const before = requests.length;
            const controller = new AbortController();
            const incremental = await client.chat.completions.create(
              { model: 'chat', messages, stream: true },
              { signal: controller.signal }
            );
            let first = '';
            for await (const chunk of incremental) {
              first += chunk.choices[0]?.delta.content ?? '';
              if (first) {
                controller.abort();
                break;
              }
            }
            assert.equal(first, 'hello');
            await Promise.race([
              upstreamCancelled,
              new Promise((_, reject) => setTimeout(() => reject(new Error('backend cancellation timed out')), 2000)),
            ]);
            assert.equal(requests.length, before + 1);
            const recovered = await client.chat.completions.create({ model: 'chat', messages });
            assert.equal(recovered.choices[0].message.content, 'hello');
          }
          if (backend === 'anthropic') {
            truncateNext = true;
            const before = requests.length;
            let partial = '';
            await assert.rejects(async () => {
              const failed = await client.chat.completions.create({ model: 'public/fallback', messages, stream: true });
              for await (const chunk of failed) partial += chunk.choices[0]?.delta.content ?? '';
            });
            assert.equal(partial, 'hello');
            assert.equal(requests.length, before + 1, 'fallback must not replay after text is visible');
          }
          assert.ok(!JSON.stringify(unary).includes('fake-key'));
        } catch (error) {
          t.diagnostic(String(error));
          throw error;
        } finally {
          proc.kill('SIGINT');
          const timer = setTimeout(() => proc.kill('SIGKILL'), 10000);
          const code = await exited;
          clearTimeout(timer);
          upstream.closeAllConnections();
          await new Promise<void>((r) => upstream.close(() => r()));
          assert.equal(code, 0, stderr);
        }
      });
    }
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
