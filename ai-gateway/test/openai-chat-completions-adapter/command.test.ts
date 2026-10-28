// Synthetic deterministic integration upstreams, not provider recordings.
import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { test } from 'node:test';
import OpenAI from 'openai';

test('official JavaScript SDK against platform plugin: Responses, compatible and Anthropic', async (t) => {
  const directory = mkdtempSync(join(tmpdir(), 'chat-completions-adapter-js-'));
  const binary = join(directory, 'gateway');
  const hostBinary = join(directory, 'plugin-host');
  try {
    execFileSync('go', ['build', '-race', '-mod=readonly', '-o', binary, './cmd/grafana-ai-gateway'], {
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
          const tool =
            !!body.tools?.length &&
            !JSON.stringify(body.messages ?? body.input).includes('tool_result') &&
            !JSON.stringify(body.input ?? []).includes('function_call_output') &&
            !(body.messages ?? []).some((m: any) => m.role === 'tool');
          const text = body.response_format || body.text ? '{"answer":"yes"}' : 'hello';
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
          const call = {
            id: 'call_weather',
            type: 'function',
            function: { name: 'weather', arguments: '{"city":"Rio"}' },
          };
          const message = tool
            ? { role: 'assistant', content: null, tool_calls: [call] }
            : { role: 'assistant', content: text };
          const finish = tool ? 'tool_calls' : 'stop';
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
          `providers:\n  failing:\n    type: openai\n    apiKeyEnv: CHAT_COMPLETIONS_ADAPTER_JS_KEY\n    baseURL: http://127.0.0.1:${addr.port}/fail/v1\n  local:\n    type: ${backend}\n${backend === 'openai-compatible' ? '    providerName: custom-vendor\n' : ''}    apiKeyEnv: CHAT_COMPLETIONS_ADAPTER_JS_KEY\n    baseURL: ${baseURL}\nmodels:\n  public/chat:\n    name: Chat\n    primary:\n      provider: local\n      model: ${backend === 'anthropic' ? 'claude-sonnet-4-20250514' : 'gpt-4o-mini'}\n    aliases: [chat]\n  public/mixed:\n    name: Mixed\n    primary:\n      provider: failing\n      model: gpt-4.1\n    fallback:\n      - provider: local\n        model: ${backend === 'anthropic' ? 'claude-sonnet-4-20250514' : 'gpt-4o-mini'}\n${fallbackConfig}`
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
              GF_PLUGIN_CONFIG_FILE: config,
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
          if (backend === 'anthropic') {
            const warned = await client.chat.completions.create({ model: 'chat', messages, seed: 7 });
            const ext = (warned as typeof warned & { grafana: { warnings: { type: string; feature: string }[] } })
              .grafana;
            assert.ok(ext.warnings.some((w) => w.type === 'unsupported' && w.feature === 'seed'));
            let seen = false;
            const warningStream = await client.chat.completions.create({
              model: 'chat',
              messages,
              seed: 7,
              stream: true,
            });
            for await (const chunk of warningStream) {
              assert.equal(chunk.id, 'msg_private');
              assert.equal(chunk.model, 'claude-sonnet-4-20250514');
              const extension = (chunk as typeof chunk & { grafana?: { warnings?: { feature: string }[] } }).grafana;
              if (extension?.warnings?.some((w) => w.feature === 'seed')) seen = true;
            }
            assert.ok(seen);
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
            () => client.chat.completions.create({ model: 'chat', messages, n: 2 }),
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
