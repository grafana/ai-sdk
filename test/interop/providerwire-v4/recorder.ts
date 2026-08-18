import { createServer, type IncomingHttpHeaders, type Server } from "node:http";

export interface ScriptedResponse {
  status?: number;
  contentType: string;
  body: string;
}

export interface RecordedRequest {
  method: string;
  path: string;
  headers: Record<string, string>;
  body: unknown;
}

export interface Recorder {
  baseURL: string;
  requests: RecordedRequest[];
  rawHeaderValues(name: string): Array<string | undefined>;
  close(): Promise<void>;
}

const HEADER_ALLOWLIST = [
  "ai-gateway-auth-method",
  "ai-gateway-protocol-version",
  "ai-language-model-id",
  "ai-language-model-specification-version",
  "ai-language-model-streaming",
  "ai-o11y-project-id",
  "authorization",
  "content-type",
  "user-agent",
  "x-call",
  "x-collision",
  "x-configured",
  "x-vercel-ai-gateway-team",
] as const;

export async function startRecorder(responses: ScriptedResponse[]): Promise<Recorder> {
  const requests: RecordedRequest[] = [];
  const rawHeaders: IncomingHttpHeaders[] = [];
  const pendingResponses = [...responses];
  const server = createServer(async (request, response) => {
    const chunks: Buffer[] = [];
    for await (const chunk of request) {
      chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
    }
    const rawBody = Buffer.concat(chunks).toString("utf8");
    rawHeaders.push(request.headers);
    requests.push({
      method: request.method ?? "",
      path: new URL(request.url ?? "", "http://recorder.invalid").pathname,
      headers: normalizeHeaders(request.headers),
      body: JSON.parse(rawBody),
    });

    const scripted = pendingResponses.shift();
    if (scripted === undefined) {
      response.writeHead(500, { "content-type": "application/json" });
      response.end(JSON.stringify({ error: { message: "unexpected request" } }));
      return;
    }
    response.writeHead(scripted.status ?? 200, { "content-type": scripted.contentType });
    response.end(scripted.body);
  });

  await listen(server);
  const address = server.address();
  if (address === null || typeof address === "string") {
    throw new Error("recorder did not bind a TCP address");
  }

  return {
    baseURL: `http://127.0.0.1:${address.port}`,
    requests,
    rawHeaderValues: (name) => rawHeaders.map((headers) => {
      const value = headers[name.toLowerCase()];
      return Array.isArray(value) ? value.join(", ") : value;
    }),
    close: () => close(server),
  };
}

function normalizeHeaders(headers: IncomingHttpHeaders): Record<string, string> {
  const normalized: Record<string, string> = {};
  for (const name of HEADER_ALLOWLIST) {
    const value = headers[name];
    if (value === undefined) continue;
    if (name === "authorization") {
      const scheme = String(value).split(" ", 1)[0];
      normalized[name] = `${scheme} <redacted>`;
      continue;
    }
    if (name === "user-agent" || name.startsWith("ai-o11y-")) {
      normalized[name] = "<present>";
      continue;
    }
    normalized[name] = Array.isArray(value) ? value.join(", ") : value;
  }
  return normalized;
}

function listen(server: Server): Promise<void> {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      server.off("error", reject);
      resolve();
    });
  });
}

function close(server: Server): Promise<void> {
  return new Promise((resolve, reject) => {
    server.close((error) => (error === undefined ? resolve() : reject(error)));
  });
}
