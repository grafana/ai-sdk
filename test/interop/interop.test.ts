import { describe, expect, it } from "vitest";
import { generateText, streamText, tool, stepCountIs } from "ai";
import { z } from "zod";
import { newGateway } from "./helpers";

// Bidirectional upstream-client conformance: a stock upstream @ai-sdk/gateway +
// ai client drives mock Go models through the public gateway/providerwire
// server and asserts two-way compatibility.

describe("upstream @ai-sdk/gateway <-> Go provider-wire", () => {
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

  it("preserves required empty delta fields through the gateway and ai client", async () => {
    const gateway = newGateway();
    const { stream } = await gateway("empty-deltas").doStream({
      prompt: [{ role: "user", content: [{ type: "text", text: "emit empty deltas" }] }],
    });

    const emptyDeltas: Array<{
      type: string;
      id: string;
      delta: string;
      hasDelta: boolean;
      providerMetadata: unknown;
    }> = [];
    for await (const part of stream) {
      if (
        (part.type === "text-delta" ||
          part.type === "reasoning-delta" ||
          part.type === "tool-input-delta") &&
        part.delta === ""
      ) {
        emptyDeltas.push({
          type: part.type,
          id: part.id,
          delta: part.delta,
          hasDelta: Object.prototype.hasOwnProperty.call(part, "delta"),
          providerMetadata: part.providerMetadata,
        });
      }
    }

    expect(emptyDeltas).toEqual([
      {
        type: "reasoning-delta",
        id: "r0",
        delta: "",
        hasDelta: true,
        providerMetadata: { interop: { empty: true } },
      },
      {
        type: "text-delta",
        id: "t0",
        delta: "",
        hasDelta: true,
        providerMetadata: { interop: { empty: true } },
      },
      {
        type: "tool-input-delta",
        id: "call_empty_1",
        delta: "",
        hasDelta: true,
        providerMetadata: { interop: { empty: true } },
      },
    ]);

    const result = streamText({
      model: gateway("empty-deltas"),
      maxRetries: 0,
      tools: {
        echoTool: tool({ inputSchema: z.object({ text: z.string() }) }),
      },
      prompt: "consume empty deltas",
    });
    const consumedTypes: string[] = [];
    for await (const part of result.fullStream) {
      consumedTypes.push(part.type);
    }

    expect(consumedTypes).toContain("text-delta");
    expect(consumedTypes).toContain("reasoning-delta");
    expect(consumedTypes).toContain("tool-input-delta");
    expect(await result.text).toBe("done");
    expect(await result.reasoningText).toBe("thought");
    expect(await result.finishReason).toBe("tool-calls");
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

  it("surfaces a provider-executed tool result value", async () => {
    const gateway = newGateway();
    const webSearch = tool({
      description: "Provider-executed web search.",
      inputSchema: z.object({ query: z.string() }),
    });

    const result = streamText({
      model: gateway("provider-tool-result"),
      maxRetries: 0,
      tools: { webSearch },
      prompt: "run a provider-executed tool",
    });

    const toolResults: Array<{ output: unknown; providerMetadata: unknown }> = [];
    for await (const part of result.fullStream) {
      if (part.type === "tool-result") {
        toolResults.push({
          output: part.output,
          providerMetadata: part.providerMetadata,
        });
      }
    }

    expect(toolResults).toEqual([
      {
        output: "Grafana is an observability platform",
        providerMetadata: { "grafana-ai-sdk": { customer: "keep" } },
      },
    ]);
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

  it("receives a file part in the response stream", async () => {
    const gateway = newGateway();
    const result = streamText({
      model: gateway("file-output"),
      maxRetries: 0,
      prompt: "generate a file",
    });

    const files: Array<{ mediaType: string }> = [];
    for await (const part of result.fullStream) {
      if (part.type === "file") {
        files.push({ mediaType: (part as { file: { mediaType: string } }).file.mediaType });
      }
    }
    expect(files.length).toBeGreaterThan(0);
    expect(files[0].mediaType).toBe("image/png");
  });

  it("preserves a URL-valued file part in the response stream", async () => {
    const gateway = newGateway();
    const result = streamText({
      model: gateway("file-output-url"),
      maxRetries: 0,
      prompt: "generate a file URL",
    });

    const files: Array<{ type: string; mediaType: string; base64: string }> = [];
    for await (const part of result.fullStream) {
      if (part.type === "file" || part.type === "reasoning-file") {
        const file = part.file;
        files.push({ type: part.type, mediaType: file.mediaType, base64: file.base64 });
      }
    }

    expect(files).toEqual([
      { type: "file", mediaType: "image/png", base64: "https://example.com/generated.png" },
      { type: "reasoning-file", mediaType: "image/png", base64: "https://example.com/reasoning.png" },
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
    expect(JSON.stringify(parts[4].error)).toContain("boom mid-stream");
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
    // Not the generic "Invalid error response format" fallback.
    expect(message).toContain("rate limited pre-stream");
  });
});
