// Deterministic Anthropic Messages stand-in for the end-to-end suite. It is
// test infrastructure, not provider evidence. A call through a configured
// endpoint returns the same fixed text, unary or streamed. A call to the
// native endpoint, which arrives through the native proxy, answers with the
// first characters of the SHA-256 of the API key it received and the backend
// model, so tests can check which key and model reached the provider without
// the key appearing anywhere.
//
// It also listens with TLS, with the certificate scripts/e2e.sh generates, as
// the custom endpoint of stack catalogs. Those calls answer with the key
// digest, the backend model and how many requests arrived for the
// egress-refused network name, which the egress policy must never connect to.
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { createServer as createSecureServer } from 'node:https';

const port = Number(process.env.PORT ?? 8080);
const securePort = Number(process.env.TLS_PORT ?? 8443);
const certificateDirectory = '/etc/native-proxy';
const model = 'fake-model';
const fixedText = ['fake ', 'stream'];
const nativeHost = 'api.anthropic.com';
const refusedHost = 'egress-refused';
const keyDigestLength = 12;
const requestsByHost = new Map();

function keyDigest(request) {
  const key = String(request.headers['x-api-key'] ?? '');
  return createHash('sha256').update(key).digest('hex').slice(0, keyDigestLength);
}

function replyText(request, body) {
  if (request.socket.encrypted) {
    return [`custom ${keyDigest(request)} `, `${body.model} ${refusedHost}=${requestsByHost.get(refusedHost) ?? 0}`];
  }
  if (request.headers.host !== nativeHost) {
    return fixedText;
  }
  return [`native ${keyDigest(request)} `, String(body.model)];
}

function unary(response, text) {
  const usage = { input_tokens: 1, output_tokens: text.length };
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

function stream(response, text) {
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

function handle(request, response) {
  const host = String(request.headers.host ?? '').replace(/:\d+$/, '');
  requestsByHost.set(host, (requestsByHost.get(host) ?? 0) + 1);
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
    const text = replyText(request, body);
    if (body.stream === true) {
      stream(response, text);
    } else {
      unary(response, text);
    }
  });
}

createServer(handle).listen(port);
createSecureServer(
  { cert: readFileSync(`${certificateDirectory}/cert.pem`), key: readFileSync(`${certificateDirectory}/key.pem`) },
  handle
).listen(securePort);
