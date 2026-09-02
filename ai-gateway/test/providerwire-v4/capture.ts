import assert from "node:assert/strict";
import type { GatewayProviderSettings } from "@ai-sdk/gateway";

const protocolHeaders = [
  "ai-language-model-specification-version",
  "ai-language-model-id",
  "ai-language-model-streaming",
] as const;

export type SemanticRequest = {
  method: string;
  path: string;
  headers: Record<string, string>;
  streaming: boolean;
  hasSignal: boolean;
  body: unknown;
};

export type CaptureFetch = {
  fetch: NonNullable<GatewayProviderSettings["fetch"]>;
  requests: SemanticRequest[];
  signals: Array<AbortSignal | null | undefined>;
};

function selectedHeaders(
  headers: Headers,
  additionalHeaderNames: ReadonlySet<string>,
): Record<string, string> {
  return Object.fromEntries(
    [...headers.entries()].filter(
      ([name]) =>
        name === "content-type" ||
        protocolHeaders.includes(name as (typeof protocolHeaders)[number]) ||
        name.startsWith("x-contract-") ||
        additionalHeaderNames.has(name),
    ),
  );
}

function streamSuccessResponse(): Response {
  const body = [
    'data: {"type":"stream-start","warnings":[]}',
    "",
    'data: {"type":"finish","usage":{"inputTokens":{},"outputTokens":{}},"finishReason":{"unified":"stop"}}',
    "",
    "",
  ].join("\n");
  return new Response(body, {
    status: 200,
    headers: { "content-type": "text/event-stream" },
  });
}

function unarySuccessResponse(): Response {
  return Response.json({
    content: [],
    finishReason: { unified: "stop" },
    usage: { inputTokens: {}, outputTokens: {} },
    warnings: [],
  });
}

export function createCaptureFetch({
  additionalHeaderNames = [],
}: {
  additionalHeaderNames?: string[];
} = {}): CaptureFetch {
  const requests: SemanticRequest[] = [];
  const signals: Array<AbortSignal | null | undefined> = [];
  const normalizedAdditionalHeaderNames = new Set(
    additionalHeaderNames.map((name) => name.toLowerCase()),
  );
  const fetch: NonNullable<GatewayProviderSettings["fetch"]> = async (input, init) => {
    if (typeof input !== "string") {
      throw new Error("capture fetch requires a string URL");
    }
    if (init?.method !== "POST" || typeof init.body !== "string") {
      throw new Error("capture fetch requires a POST request with a string body");
    }

    const rawHeaders = Object.entries(init.headers as Record<string, string>);
    for (const name of protocolHeaders) {
      assert.equal(
        rawHeaders.filter(([candidate]) => candidate.toLowerCase() === name).length,
        1,
        `${name} must have one effective value`,
      );
    }

    const headers = new Headers(init.headers);
    const streaming = headers.get("ai-language-model-streaming") === "true";
    signals.push(init.signal);
    requests.push({
      method: init.method,
      path: new URL(input).pathname,
      headers: selectedHeaders(headers, normalizedAdditionalHeaderNames),
      streaming,
      hasSignal: init.signal != null,
      body: JSON.parse(init.body),
    });

    return streaming ? streamSuccessResponse() : unarySuccessResponse();
  };

  return { fetch, requests, signals };
}

export async function drainStream(stream: ReadableStream<unknown>): Promise<void> {
  const reader = stream.getReader();
  while (!(await reader.read()).done) {}
}
