import { describe, expect, it } from "vitest";
import { generateText, streamText, tool, stepCountIs } from "ai";
import { z } from "zod";
import { newGateway as newGatewayFor, type ProviderWireEndpoint } from "./helpers";

// Bidirectional upstream-client conformance: a stock upstream @ai-sdk/gateway +
// ai client drives mock Go models through the public gateway/providerwire
// server and asserts two-way compatibility.

describe.each(["legacy", "strict"] as const)("upstream @ai-sdk/gateway <-> Go %s provider-wire", (endpoint: ProviderWireEndpoint) => {
  const newGateway = () => newGatewayFor(endpoint);

  it("generates a rich unary result while owning transport metadata", async () => {
    const gateway = newGateway();
    const result = await gateway("generate-rich").doGenerate({
      prompt: [{ role: "user", content: [{ type: "text", text: "generate everything" }] }],
    });

    expect(result.content.map((part) => part.type)).toEqual([
      "text",
      "reasoning",
      "tool-call",
      "tool-result",
      "file",
      "source",
    ]);
    expect(result.content[2]).toMatchObject({
      type: "tool-call",
      toolCallId: "call_1",
      toolName: "lookup",
      input: '{"query":"grafana"}',
    });
    expect(result.content[3]).toMatchObject({
      type: "tool-result",
      toolCallId: "call_1",
      result: { answer: 42 },
    });
    expect(result.content[4]).toMatchObject({
      type: "file",
      mediaType: "application/octet-stream",
      data: { type: "data", data: "AAEC" },
    });
    expect(result.content[5]).toMatchObject({
      type: "source",
      sourceType: "url",
      id: "source_1",
      url: "https://example.com/source",
    });
    expect(result.finishReason).toEqual({ unified: "stop", raw: "end_turn" });
    expect(result.usage).toMatchObject({
      inputTokens: { total: 11, noCache: 7, cacheRead: 4 },
      outputTokens: { total: 6, text: 4, reasoning: 2 },
    });
    expect(result.providerMetadata).toEqual({ interop: { trace: "public" } });

    // The pinned gateway client replaces server warning/request/response fields
    // with values owned by its own HTTP transport.
    expect(result.warnings).toEqual([]);
    expect(result.request.body).toMatchObject({ prompt: expect.any(Array) });
    expect(result.request.body).not.toEqual({ serverRequest: "private" });
    expect(result.response.headers).toHaveProperty("content-type");
    expect(result.response.headers).not.toHaveProperty("x-backend-secret");
    expect(result.response.body).toBeDefined();
  });

  it("encodes canonical Uint8Array tool-result file data", async () => {
    const fileData = new Uint8Array([0, 1, 2]);
    const gateway = newGateway();
    const result = await gateway("tool-result-file-input").doGenerate({
      prompt: [
        {
          role: "tool",
          content: [
            {
              type: "tool-result",
              toolCallId: "call_file_1",
              toolName: "readFile",
              output: {
                type: "content",
                value: [
                  {
                    type: "file",
                    data: { type: "data", data: fileData },
                    mediaType: "application/octet-stream",
                    filename: "input.bin",
                  },
                ],
              },
            },
          ],
        },
      ],
    });

    expect(result.content).toEqual([
      {
        type: "text",
        text: "data=AAEC mediaType=application/octet-stream filename=input.bin",
      },
    ]);
  });

  it("streams text with a system prompt", async () => {
    const gateway = newGateway();
    const result = streamText({
      model: gateway("stream-text"),
      maxRetries: 0,
      system: "be concise",
      prompt: "say hello",
    });

    let text = "";
    const types: string[] = [];
    for await (const part of result.fullStream) {
      types.push(part.type);
      if (part.type === "text-delta") {
        text += (part as { text?: string; delta?: string }).text ?? (part as { delta?: string }).delta ?? "";
      }
    }

    // System prompt round-tripped as a string and was decoded server-side.
    expect(text).toContain("system=be concise");
    expect(text).toContain("hello from go");
    expect(await result.finishReason).toBe("stop");
    const usage = await result.usage;
    expect(usage.inputTokens).toBe(10);
    expect(types).not.toContain("error");
  });

  it("round-trips a client-executed tool call", async () => {
    const gateway = newGateway();
    let executedWith: unknown;
    const echoTool = tool({
      description: "Echoes text back.",
      inputSchema: z.object({ text: z.string() }),
      execute: async (input: { text: string }) => {
        executedWith = input;
        return { echoed: input.text };
      },
    });

    const result = streamText({
      model: gateway("tool-call"),
      maxRetries: 0,
      tools: { echoTool },
      stopWhen: stepCountIs(5),
      prompt: "call the echoTool",
    });

    const toolCalls: Array<{ toolName: string; input: unknown }> = [];
    for await (const part of result.fullStream) {
      if (part.type === "tool-call") {
        toolCalls.push({ toolName: (part as { toolName: string }).toolName, input: (part as { input: unknown }).input });
      }
    }

    const finalText = await result.text;
    expect(toolCalls[0]?.toolName).toBe("echoTool");
    expect(executedWith).toEqual({ text: "hello from go tool call" });
    // The second request carried the upstream-shaped tool-result, which the Go
    // server decoded and echoed back.
    expect(finalText).toContain("done:");
    expect(finalText).toContain("echoTool");
  });

  it("decodes an upstream file input part", async () => {
    const gateway = newGateway();
    const pngBase64 =
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==";

    const result = streamText({
      model: gateway("file-input"),
      maxRetries: 0,
      messages: [
        {
          role: "user",
          content: [
            { type: "text", text: "What is in this image?" },
            { type: "file", data: pngBase64, mediaType: "image/png", filename: "pixel.png" },
          ],
        },
      ],
    });

    const text = await result.text;
    expect(text).toContain("decoded 1 file part");
    expect(text).toMatch(/base64Len=[1-9]/);
  });

  it("streams URL and document sources", async () => {
    const gateway = newGateway();
    const { stream } = await gateway("stream-sources").doStream({
      prompt: [{ role: "user", content: [{ type: "text", text: "cite sources" }] }],
    });

    const sources: unknown[] = [];
    for await (const part of stream) {
      if (part.type === "source") sources.push(part);
    }
    expect(sources).toEqual([
      {
        type: "source",
        sourceType: "url",
        id: "url-source",
        url: "https://example.com",
        title: "Example",
      },
      {
        type: "source",
        sourceType: "document",
        id: "document-source",
        title: "Document",
        mediaType: "application/pdf",
        filename: "document.pdf",
      },
    ]);
  });

  it("continues after a mid-stream provider error", async () => {
    const gateway = newGateway();
    const { stream } = await gateway("error-mid-stream").doStream({
      prompt: [{ role: "user", content: [{ type: "text", text: "trigger a mid-stream error" }] }],
    });

    const parts: Array<{ type: string; [key: string]: unknown }> = [];
    for await (const part of stream) {
      parts.push(part as { type: string; [key: string]: unknown });
    }

    expect(parts.map((part) => part.type)).toEqual([
      "stream-start",
      "response-metadata",
      "text-start",
      "text-delta",
      "error",
      "text-delta",
      "text-end",
      "finish",
    ]);
    if (endpoint === "legacy") {
      expect(JSON.stringify(parts[4].error)).toContain("boom mid-stream");
    } else {
      expect(JSON.stringify(parts[4].error)).not.toContain("boom mid-stream");
      expect(JSON.stringify(parts[4].error)).toContain("upstream dependency failed");
    }
    expect(parts[5].delta).toBe("continued after error");
    expect(parts[7].finishReason).toEqual({ unified: "error", raw: "error" });
  });

  it("surfaces a pre-stream HTTP error with the server message", async () => {
    const gateway = newGateway();
    let message = "";
    try {
      await generateText({
        model: gateway("error-pre-stream"),
        maxRetries: 0,
        prompt: "trigger a pre-stream error",
      });
    } catch (e) {
      message = (e as Error).message ?? String(e);
    }
    // Not the generic "Invalid error response format" fallback. Strict mode
    // intentionally redacts provider detail to its stable category message.
    expect(message).toContain(endpoint === "legacy" ? "rate limited pre-stream" : "rate limit exceeded");
  });
});

describe("strict provider-wire HTTP retry behavior", () => {
  it("keeps the pinned HTTP-500 retry asymmetry", async () => {
    const gateway = newGatewayFor("strict");
    let observed: { statusCode?: number; type?: string; isRetryable?: boolean } = {};
    try {
      await gateway("strict-error-internal").doGenerate({ prompt: [] });
    } catch (error) {
      observed = error as typeof observed;
    }

    expect(observed.statusCode).toBe(500);
    expect(observed.type).toBe("internal_server_error");
    expect(observed.isRetryable).toBe(true);
  });
});
