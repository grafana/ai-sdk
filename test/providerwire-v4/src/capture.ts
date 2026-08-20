import { createServer, type IncomingHttpHeaders } from "node:http";
import type { AddressInfo } from "node:net";
import type {
  LanguageModelV4,
  LanguageModelV4CallOptions,
  LanguageModelV4StreamPart,
} from "@ai-sdk/provider";
import { createGateway, type GatewayProviderSettings } from "@ai-sdk/gateway";
import { parseStrictJson } from "./strict-json.ts";

export type EnvelopeClassification = "supported" | "unsupported-reserved-collision";

export interface SemanticRequest {
  sequence: number;
  method: string;
  path: string;
  requestPath: string;
  headers: Record<string, string>;
  envelope: EnvelopeClassification;
  body: unknown;
}

export interface ScenarioCapture {
  name: string;
  requests: SemanticRequest[];
}

interface ScenarioOptions {
  mode?: "generate" | "stream";
  gatewayHeaders?: Record<string, string>;
  callOptions?: LanguageModelV4CallOptions;
  execute?: (model: LanguageModelV4) => Promise<void>;
  streamParts?: (sequence: number) => LanguageModelV4StreamPart[];
}

const modelId = "provider/model-v4-evidence";
const protocolHeaders = [
  "ai-language-model-id",
  "ai-language-model-specification-version",
  "ai-language-model-streaming",
] as const;

export async function captureScenario(name: string, options: ScenarioOptions): Promise<ScenarioCapture> {
  const requests: SemanticRequest[] = [];
  const server = createServer((request, response) => {
    const chunks: Buffer[] = [];
    request.on("data", (chunk: Buffer) => chunks.push(chunk));
    request.on("end", () => {
      try {
        const rawBody = Buffer.concat(chunks).toString("utf8");
        const body = parseStrictJson(rawBody);
        const headers = semanticHeaders(request.headers);
        requests.push({
          sequence: requests.length + 1,
          method: request.method ?? "",
          path: relativeLanguageModelPath(request.url ?? ""),
          requestPath: request.url ?? "",
          headers,
          envelope: classifyEnvelope(headers),
          body,
        });
        if (headers["ai-language-model-streaming"] === "true") {
          response.writeHead(200, { "Content-Type": "text/event-stream" });
          const parts = options.streamParts?.(requests.length) ?? [finishPart()];
          response.end(parts.map((part) => `data: ${JSON.stringify(part)}\n\n`).join(""));
          return;
        }
        response.writeHead(200, { "Content-Type": "application/json" });
        response.end(JSON.stringify(generateResult()));
      } catch (error) {
        response.writeHead(500, { "Content-Type": "application/json" });
        response.end(JSON.stringify({ error: String(error) }));
      }
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address() as AddressInfo;
  const settings: GatewayProviderSettings = {
    baseURL: `http://127.0.0.1:${port}/api/v1/aisdk`,
    apiKey: "providerwire-v4-evidence-key",
    headers: options.gatewayHeaders,
  };
  try {
    const model = createGateway(settings)(modelId);
    if (options.execute) {
      await options.execute(model);
    } else if (options.mode === "stream" && options.callOptions) {
      const result = await model.doStream(options.callOptions);
      for await (const _part of result.stream) {
        // Consumption is required so the client completes the response body.
      }
    } else if (options.callOptions) {
      await model.doGenerate(options.callOptions);
    } else {
      throw new Error(`${name} requires callOptions or execute`);
    }
  } finally {
    await new Promise<void>((resolve, reject) =>
      server.close((error) => (error ? reject(error) : resolve())),
    );
  }
  return { name, requests };
}

function relativeLanguageModelPath(requestPath: string): string {
  const prefix = "/api/v1/aisdk";
  if (!requestPath.startsWith(prefix)) {
    throw new Error(`request path ${requestPath} does not use configured baseURL prefix ${prefix}`);
  }
  return requestPath.slice(prefix.length);
}

function semanticHeaders(headers: IncomingHttpHeaders): Record<string, string> {
  const retained: Record<string, string> = {};
  for (const [name, value] of Object.entries(headers)) {
    if (!shouldRetainHeader(name) || value === undefined) {
      continue;
    }
    const normalizedValue = Array.isArray(value)
      ? value.join(", ")
      : typeof value === "string"
        ? value
        : String(value);
    retained[name] = normalizeClientOwnedHeader(name, normalizedValue);
  }
  return Object.fromEntries(Object.entries(retained).sort(([left], [right]) => left.localeCompare(right)));
}

const transportOnlyHeaders = new Set([
  "accept",
  "accept-encoding",
  "accept-language",
  "connection",
  "content-length",
  "host",
  "sec-fetch-mode",
]);

function shouldRetainHeader(name: string): boolean {
  return !transportOnlyHeaders.has(name);
}

function normalizeClientOwnedHeader(name: string, value: string): string {
  if (name === "authorization") {
    return "<client-owned-authorization>";
  }
  if (name === "user-agent") {
    return "<client-owned-user-agent>";
  }
  return value;
}

export function classifyEnvelope(headers: Record<string, string>): EnvelopeClassification {
  if (headers["content-type"] !== "application/json") {
    return "unsupported-reserved-collision";
  }
  const expected: Record<(typeof protocolHeaders)[number], string | undefined> = {
    "ai-language-model-id": modelId,
    "ai-language-model-specification-version": "4",
    "ai-language-model-streaming": headers["ai-language-model-streaming"],
  };
  if (!['true', 'false'].includes(headers["ai-language-model-streaming"] ?? "")) {
    return "unsupported-reserved-collision";
  }
  for (const name of protocolHeaders) {
    const value = headers[name];
    if (!value || value !== expected[name] || value.includes(",")) {
      return "unsupported-reserved-collision";
    }
  }
  return "supported";
}

function generateResult() {
  return {
    content: [{ type: "text", text: "ok" }],
    finishReason: { unified: "stop", raw: "stop" },
    usage: {
      inputTokens: { total: 1, noCache: 1, cacheRead: 0, cacheWrite: 0 },
      outputTokens: { total: 1, text: 1, reasoning: 0 },
    },
  };
}

function finishPart(): LanguageModelV4StreamPart {
  return {
    type: "finish",
    finishReason: { unified: "stop", raw: "stop" },
    usage: {
      inputTokens: { total: 1, noCache: 1, cacheRead: 0, cacheWrite: 0 },
      outputTokens: { total: 1, text: 1, reasoning: 0 },
    },
  };
}
