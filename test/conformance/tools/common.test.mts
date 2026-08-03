import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { safeValidateTypes } from "@ai-sdk/provider-utils";
import {
  buildMessages,
  buildStreamTextOptions,
  buildToolChoice,
  buildTools,
  createSourceIdNormalizer,
} from "./common.mts";

describe("conformance common config", () => {
  it("normalizes source IDs consistently", () => {
    const normalize = createSourceIdNormalizer();

    assert.deepEqual(
      normalize({ type: "source-document", sourceId: "random-a", filename: "report.pdf" }),
      { type: "source-document", sourceId: "src-0", filename: "report.pdf" },
    );
    assert.deepEqual(
      normalize({ type: "source-url", sourceId: "random-a", url: "https://example.com" }),
      { type: "source-url", sourceId: "src-0", url: "https://example.com" },
    );
    assert.deepEqual(normalize({ type: "source-document", sourceId: "random-b" }), {
      type: "source-document",
      sourceId: "src-1",
    });
    assert.deepEqual(normalize({ type: "text-delta", id: "text-1" }), {
      type: "text-delta",
      id: "text-1",
    });
  });

  it("builds tool choice configs", () => {
    assert.equal(buildToolChoice({ model: "m", toolChoice: { type: "required" } }), "required");
    assert.deepEqual(
      buildToolChoice({ model: "m", toolChoice: { type: "tool", toolName: "weather" } }),
      { type: "tool", toolName: "weather" },
    );
  });

  it("builds provider tool provider options and input schema", async () => {
    const tools = buildTools(undefined, {
      web_search: {
        id: "anthropic.web_search_20250305",
        args: { maxUses: 1 },
        inputSchema: {
          type: "object",
          properties: { query: { type: "string" } },
          required: ["query"],
        },
        providerOptions: { anthropic: { deferLoading: true } },
      },
    });

    assert.deepEqual(tools?.web_search.providerOptions, {
      anthropic: { deferLoading: true },
    });
    assert.equal(
      (await safeValidateTypes({ value: { query: "weather" }, schema: tools!.web_search.inputSchema })).success,
      true,
    );
    assert.equal(
      (await safeValidateTypes({ value: {}, schema: tools!.web_search.inputSchema })).success,
      false,
    );
  });

  it("builds function tool execution errors", async () => {
    const tools = buildTools(
      {
        weather: {
          description: "weather",
          inputSchema: { type: "object" },
          mockError: "boom",
        },
      },
      undefined,
    );

    const execute = tools!.weather.execute!;
    await assert.rejects(() => execute({}, {} as Parameters<typeof execute>[1]), /boom/);
  });

  it("preserves explicit false function tool strict settings", () => {
    const tools = buildTools(
      {
        weather: {
          description: "weather",
          inputSchema: { type: "object" },
          strict: false,
        },
      },
      undefined,
    );

    assert.equal(tools?.weather.strict, false);
  });

  it("builds active tool stream text options", () => {
    const options = buildStreamTextOptions(
      {
        model: "m",
        activeTools: ["search", "weather"],
      },
      {
        model: {},
        prompt: "test",
        stopWhen: {},
      },
    );

    assert.deepEqual(options.activeTools, ["search", "weather"]);
  });

  it("builds configured provider-reference file parts", () => {
    const messages = buildMessages(
      {
        model: "m",
        messages: [
          {
            role: "user",
            content: [
              {
                type: "file",
                mediaType: "application/pdf",
                filename: "doc.pdf",
                reference: { openai: "file-abc123" },
              },
            ],
          },
        ],
      },
      "ignored",
    );

    assert.deepEqual(messages, [
      {
        role: "user",
        content: [
          {
            type: "file",
            mediaType: "application/pdf",
            filename: "doc.pdf",
            data: { type: "reference", reference: { openai: "file-abc123" } },
          },
        ],
      },
    ]);
  });

  it("builds configured messages with part provider options", () => {
    const messages = buildMessages(
      {
        model: "m",
        messages: [
          {
            role: "assistant",
            content: [
              {
                type: "reasoning",
                text: "thinking",
                providerOptions: { openai: { itemId: "rs_prev" } },
              },
            ],
          },
          { role: "user", content: "continue" },
        ],
      },
      "ignored",
    );

    assert.deepEqual(messages, [
      {
        role: "assistant",
        content: [
          {
            type: "reasoning",
            text: "thinking",
            providerOptions: { openai: { itemId: "rs_prev" } },
          },
        ],
      },
      { role: "user", content: "continue" },
    ]);
  });

  it("builds configured tool calls", () => {
    const messages = buildMessages(
      {
        model: "m",
        messages: [
          {
            role: "assistant",
            content: [
              {
                type: "tool-call",
                toolCallId: "call-1",
                toolName: "$READFILE",
                input: { path: "/tmp/file" },
              },
            ],
          },
        ],
      },
      "ignored",
    );

    assert.deepEqual(messages, [
      {
        role: "assistant",
        content: [
          {
            type: "tool-call",
            toolCallId: "call-1",
            toolName: "$READFILE",
            input: { path: "/tmp/file" },
          },
        ],
      },
    ]);
  });

  it("builds configured file parts", () => {
    const messages = buildMessages(
      {
        model: "m",
        messages: [
          {
            role: "user",
            content: [
              {
                type: "file",
                data: "AAECAw==",
                mediaType: "application/pdf",
                filename: "report.pdf",
              },
            ],
          },
        ],
      },
      "ignored",
    );

    assert.deepEqual(messages, [
      {
        role: "user",
        content: [
          {
            type: "file",
            data: "AAECAw==",
            mediaType: "application/pdf",
            filename: "report.pdf",
          },
        ],
      },
    ]);

    const urlMessages = buildMessages(
      {
        model: "m",
        messages: [
          {
            role: "user",
            content: [
              {
                type: "file",
                url: "s3://bucket/image.png",
                mediaType: "image/png",
              },
            ],
          },
        ],
      },
      "ignored",
    );
    const file = (urlMessages?.[0] as { content: Array<{ data: URL }> }).content[0];
    assert.equal(file.data.href, "s3://bucket/image.png");
  });
});
