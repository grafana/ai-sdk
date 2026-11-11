// Deterministic Anthropic Messages stand-in for the end-to-end suite. It is
// test infrastructure, not provider evidence: every call returns the same
// fixed text, unary or streamed, except a unary call whose last user text is
// the echo marker, which returns the JSON of the request fields that
// Middleware call defaults change.
import { createServer } from 'node:http';

const port = Number(process.env.PORT ?? 8080);
const model = 'fake-model';
const text = ['fake ', 'stream'];

const usage = { input_tokens: 1, output_tokens: text.length };
const echoMarker = 'e2e-echo-request';

function lastUserText(body) {
  const last = body.messages?.at(-1);
  if (last?.role !== 'user') {
    return undefined;
  }
  return typeof last.content === 'string' ? last.content : last.content?.at(-1)?.text;
}

function echoed(body) {
  return JSON.stringify({
    system: body.system ?? null,
    temperature: body.temperature ?? null,
    max_tokens: body.max_tokens ?? null,
    stop_sequences: body.stop_sequences ?? null,
  });
}

function unary(response, content = text.join('')) {
  response.writeHead(200, { 'Content-Type': 'application/json' });
  response.end(
    JSON.stringify({
      id: 'msg_fake',
      type: 'message',
      role: 'assistant',
      model,
      content: [{ type: 'text', text: content }],
      stop_reason: 'end_turn',
      stop_sequence: null,
      usage,
    })
  );
}

function stream(response) {
  response.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' });
  const event = (name, data) => response.write(`event: ${name}\ndata: ${JSON.stringify({ type: name, ...data })}\n\n`);
  event('message_start', {
    message: {
      id: 'msg_fake',
      type: 'message',
      role: 'assistant',
      content: [],
      model,
      stop_reason: null,
      stop_sequence: null,
      usage: { input_tokens: 1, output_tokens: 0 },
    },
  });
  event('content_block_start', { index: 0, content_block: { type: 'text', text: '' } });
  for (const part of text) {
    event('content_block_delta', { index: 0, delta: { type: 'text_delta', text: part } });
  }
  event('content_block_stop', { index: 0 });
  event('message_delta', {
    delta: { stop_reason: 'end_turn', stop_sequence: null },
    usage: { output_tokens: text.length },
  });
  event('message_stop', {});
  response.end();
}

createServer((request, response) => {
  if (request.method !== 'POST' || !request.url?.endsWith('/v1/messages')) {
    response.writeHead(404).end();
    return;
  }
  const chunks = [];
  request.on('data', (chunk) => chunks.push(chunk));
  request.on('end', () => {
    let body;
    try {
      body = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    } catch {
      response.writeHead(400).end();
      return;
    }
    if (body.stream === true) {
      stream(response);
    } else if (lastUserText(body) === echoMarker) {
      unary(response, echoed(body));
    } else {
      unary(response);
    }
  });
}).listen(port);
