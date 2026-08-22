import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";
import { compileDefinition, formatValidationErrors } from "./schema.ts";

type Specimen = {
  value: Record<string, unknown>;
  required: string[];
};

const specimens: Record<string, Specimen> = {
  fileDataData: { value: { type: "data", data: "" }, required: ["type", "data"] },
  fileDataUrl: {
    value: { type: "url", url: "https://example.com/%" },
    required: ["type", "url"],
  },
  fileDataReference: {
    value: { type: "reference", reference: { provider: "file" } },
    required: ["type", "reference"],
  },
  fileDataText: { value: { type: "text", text: "" }, required: ["type", "text"] },
  textPart: { value: { type: "text", text: "" }, required: ["type", "text"] },
  reasoningPart: {
    value: { type: "reasoning", text: "" },
    required: ["type", "text"],
  },
  customPart: {
    value: { type: "custom", kind: "provider.kind" },
    required: ["type", "kind"],
  },
  filePart: {
    value: { type: "file", data: { type: "data", data: "" }, mediaType: "text/plain" },
    required: ["type", "data", "mediaType"],
  },
  reasoningFilePart: {
    value: {
      type: "reasoning-file",
      data: { type: "data", data: "" },
      mediaType: "text/plain",
    },
    required: ["type", "data", "mediaType"],
  },
  toolCallPart: {
    value: { type: "tool-call", toolCallId: "call", toolName: "tool", input: null },
    required: ["type", "toolCallId", "toolName", "input"],
  },
  toolResultPart: {
    value: {
      type: "tool-result",
      toolCallId: "call",
      toolName: "tool",
      output: { type: "execution-denied" },
    },
    required: ["type", "toolCallId", "toolName", "output"],
  },
  toolApprovalResponsePart: {
    value: { type: "tool-approval-response", approvalId: "approval", approved: false },
    required: ["type", "approvalId", "approved"],
  },
  toolResultTextOutput: {
    value: { type: "text", value: "" },
    required: ["type", "value"],
  },
  toolResultJsonOutput: {
    value: { type: "json", value: null },
    required: ["type", "value"],
  },
  toolResultDeniedOutput: {
    value: { type: "execution-denied" },
    required: ["type"],
  },
  toolResultErrorTextOutput: {
    value: { type: "error-text", value: "" },
    required: ["type", "value"],
  },
  toolResultErrorJsonOutput: {
    value: { type: "error-json", value: null },
    required: ["type", "value"],
  },
  toolResultContentOutput: {
    value: { type: "content", value: [] },
    required: ["type", "value"],
  },
  toolResultContentCustom: {
    value: { type: "custom" },
    required: ["type"],
  },
  systemMessage: {
    value: { role: "system", content: "" },
    required: ["role", "content"],
  },
  userMessage: {
    value: { role: "user", content: [] },
    required: ["role", "content"],
  },
  assistantMessage: {
    value: { role: "assistant", content: [] },
    required: ["role", "content"],
  },
  toolMessage: {
    value: { role: "tool", content: [] },
    required: ["role", "content"],
  },
  functionTool: {
    value: { type: "function", name: "tool", inputSchema: {} },
    required: ["type", "name", "inputSchema"],
  },
  providerTool: {
    value: { type: "provider", id: "provider.tool", name: "tool", args: {} },
    required: ["type", "id", "name", "args"],
  },
  inputExample: {
    value: { input: {} },
    required: ["input"],
  },
  toolChoiceAuto: {
    value: { type: "auto" },
    required: ["type"],
  },
  toolChoiceNone: {
    value: { type: "none" },
    required: ["type"],
  },
  toolChoiceRequired: {
    value: { type: "required" },
    required: ["type"],
  },
  toolChoiceNamed: {
    value: { type: "tool", toolName: "tool" },
    required: ["type", "toolName"],
  },
  responseFormatText: {
    value: { type: "text" },
    required: ["type"],
  },
  responseFormatJson: {
    value: { type: "json" },
    required: ["type"],
  },
};

const schema = JSON.parse(
  readFileSync(new URL("../../gateway/providerwire/v4/schema/request.json", import.meta.url), "utf8"),
) as {
  $defs: Record<string, { type?: string; required?: string[]; additionalProperties?: unknown }>;
};

describe("finite request object definitions", () => {
  it("has a specimen for every closed object definition", () => {
    const closedDefinitions = Object.entries(schema.$defs)
      .filter(([, definition]) => definition.type === "object" && definition.additionalProperties === false)
      .map(([name]) => name)
      .sort();

    assert.deepEqual(Object.keys(specimens).sort(), closedDefinitions);
  });

  for (const [name, specimen] of Object.entries(specimens)) {
    it(`enforces ${name} closure and required members`, () => {
      const definition = schema.$defs[name];
      assert.deepEqual([...(definition.required ?? [])].sort(), [...specimen.required].sort());

      const validate = compileDefinition(name);
      assert.equal(validate(specimen.value), true, formatValidationErrors(validate.errors));
      assert.equal(validate({ ...specimen.value, unknown: true }), false);

      for (const property of specimen.required) {
        const missing = structuredClone(specimen.value);
        delete missing[property];
        assert.equal(validate(missing), false, `${name} accepted missing ${property}`);
      }
    });
  }
});
