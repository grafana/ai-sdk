// Deterministic Anthropic Messages stand-in for the end-to-end suite. It is
// test infrastructure, not provider evidence: every call returns the same
// fixed text, unary or streamed.
import { createServer } from 'node:http';

const port = Number(process.env.PORT ?? 8080);
const model = 'fake-model';
const text = ['fake ', 'stream'];

const usage = { input_tokens: 1, output_tokens: text.length };

function unary(response) {
  response.writeHead(200, { 'Content-Type': 'application/json' });
  response.end(
    JSON.stringify({
      id: 'msg_fake',
      type: 'message',
      role: 'assistant',
      model,
      content: [{ type: 'text', text: text.join('') }],
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
    } else {
      unary(response);
    }
  });
}).listen(port);
