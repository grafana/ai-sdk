import { readFileSync, writeFileSync } from "node:fs";
import type { IncomingHttpHeaders } from "node:http";
import { join } from "node:path";
import { parse as parseYaml } from "yaml";
import { Output, jsonSchema, type ModelMessage, type Tool, type UIMessage } from "ai";
import { z } from "zod";
import type { ProviderOptions, ToolResultOutput } from "@ai-sdk/provider-utils";

// --- Types ---

export interface ToolConfig {
  description: string;
  inputSchema: Record<string, unknown>;
  mockResults?: unknown[];
  mockError?: string;
  modelOutput?: ToolModelOutputConfig;
  providerOptions?: Record<string, Record<string, unknown>>;
  needsApproval?: boolean;
  strict?: boolean;
}

export interface ToolModelOutputConfig {
  type: "text" | "content";
  text?: string;
  content?: Extract<ToolResultOutput, { type: "content" }>["value"];
}

export interface ToolChoiceConfig {
  type: "auto" | "none" | "required" | "tool";
  toolName?: string;
}

export interface StreamOptionsConfig {
  sendReasoning?: boolean;
  sendSources?: boolean;
  sendFinish?: boolean;
  sendStart?: boolean;
}

export interface ApprovalConfig {
  toolCallId: string;
  toolName: string;
  approvalId: string;
  approved: boolean;
  reason?: string;
  input?: unknown;
}

export interface ProviderToolConfig {
  id: string;
  args?: Record<string, unknown>;
  inputSchema?: Record<string, unknown>;
  providerOptions?: Record<string, Record<string, unknown>>;
}

export interface ResponseFormatConfig {
  type: "json";
  outputMode?: "object" | "array" | "choice" | "json";
  schema: Record<string, unknown>;
  choices?: string[];
  name?: string;
  description?: string;
}

export interface Config {
  operation?: "stream" | "generate";
  model: string;
  system?: string;
  prompt?: string;
  messages?: MessageConfig[];
  uiMessages?: UIMessage[];
  allowSystemInMessages?: boolean;
  stopWhenStepCount?: number;
  toolChoice?: ToolChoiceConfig;
  activeTools?: string[];
  reasoning?: "provider-default" | "none" | "minimal" | "low" | "medium" | "high" | "xhigh";
  headers?: Record<string, string>;
  streamOptions?: StreamOptionsConfig;
  providerOptions?: Record<string, Record<string, unknown>>;
  tools?: Record<string, ToolConfig>;
  providerTools?: Record<string, ProviderToolConfig>;
  responseFormat?: ResponseFormatConfig;
  assertOutputValue?: boolean;
  approval?: ApprovalConfig;
  approvals?: ApprovalConfig[];
  expectStreamError?: boolean;
  maxRetries?: number;
}

export interface MessageConfig {
  role: "system" | "user" | "assistant" | "tool";
  content: string | MessagePartConfig[];
  providerOptions?: Record<string, Record<string, unknown>>;
}

export interface MessagePartConfig {
  type:
    | "text"
    | "reasoning"
    | "file"
    | "tool-call"
    | "tool-result"
    | "tool-approval-request"
    | "tool-approval-response";
  text?: string;
  data?: string;
  url?: string;
  mediaType?: string;
  filename?: string;
  reference?: Record<string, string>;
  toolCallId?: string;
  toolName?: string;
  approvalId?: string;
  input?: unknown;
  output?: unknown;
  approved?: boolean;
  reason?: string;
  isAutomatic?: boolean;
  providerExecuted?: boolean;
  providerOptions?: Record<string, Record<string, unknown>>;
}

export interface StreamTextOptionsConfig {
  model: unknown;
  messages?: ModelMessage[];
  prompt?: string;
  tools?: Record<string, Tool>;
  output?: unknown;
  stopWhen: unknown;
  generateId?: () => string;
}

export interface TestCase {
  name: string;
  dir: string;
  provider: string;
}

export interface RequestSnapshot {
  method: string;
  path: string;
  headers: Record<string, string>;
  body: unknown;
}

// --- ID Generator ---

export function mockId(prefix: string = "id"): () => string {
  let counter = 0;
  return () => `${prefix}-${counter++}`;
}

export function createSourceIdNormalizer(prefix: string = "src"): (chunk: unknown) => unknown {
  const normalizedIds = new Map<string, string>();

  return (chunk: unknown): unknown => {
    if (chunk === null || typeof chunk !== "object") return chunk;

    const record = chunk as Record<string, unknown>;
    if (
      (record.type !== "source-url" && record.type !== "source-document") ||
      typeof record.sourceId !== "string"
    ) {
      return chunk;
    }

    let normalizedId = normalizedIds.get(record.sourceId);
    if (normalizedId === undefined) {
      normalizedId = `${prefix}-${normalizedIds.size}`;
      normalizedIds.set(record.sourceId, normalizedId);
    }

    return { ...record, sourceId: normalizedId };
  };
}

// --- Config ---

export function loadConfig(dir: string): Config {
  const path = join(dir, "config.yaml");
  return parseConfig(readFileSync(path, "utf8"), path);
}

// parseConfig parses fixture YAML and rejects any key no config type declares,
// so a misspelled option fails before generation or recording instead of
// silently leaving both the snapshot and the Go replay without the behavior it
// named. Payload values such as provider options, schemas, tool inputs and UI
// message parts stay open. The Go loader applies the same rule, and
// testdata/fixture-config/all-keys.yaml keeps the two key sets aligned.
export function parseConfig(raw: string, source: string): Config {
  const value: unknown = parseYaml(raw);
  if (!isRecord(value)) {
    throw new Error(`${source}: fixture config must be a mapping`);
  }
  assertKnownKeys(value, CONFIG_KEYS, source);
  for (const [i, message] of entries(value.messages)) {
    assertKnownKeys(message, MESSAGE_KEYS, `${source}: messages[${i}]`);
    if (isRecord(message)) {
      for (const [j, part] of entries(message.content)) {
        assertKnownKeys(part, MESSAGE_PART_KEYS, `${source}: messages[${i}].content[${j}]`);
      }
    }
  }
  for (const [i, message] of entries(value.uiMessages)) {
    assertKnownKeys(message, UI_MESSAGE_KEYS, `${source}: uiMessages[${i}]`);
  }
  for (const [name, tool] of entries(value.tools)) {
    assertKnownKeys(tool, TOOL_KEYS, `${source}: tools.${name}`);
    const modelOutput = isRecord(tool) ? tool.modelOutput : undefined;
    assertKnownKeys(modelOutput, TOOL_MODEL_OUTPUT_KEYS, `${source}: tools.${name}.modelOutput`);
    const content = isRecord(modelOutput) ? modelOutput.content : undefined;
    for (const [i, item] of entries(content)) {
      assertKnownKeys(item, TOOL_MODEL_OUTPUT_CONTENT_KEYS, `${source}: tools.${name}.modelOutput.content[${i}]`);
    }
  }
  for (const [name, tool] of entries(value.providerTools)) {
    assertKnownKeys(tool, PROVIDER_TOOL_KEYS, `${source}: providerTools.${name}`);
  }
  assertKnownKeys(value.toolChoice, TOOL_CHOICE_KEYS, `${source}: toolChoice`);
  assertKnownKeys(value.streamOptions, STREAM_OPTIONS_KEYS, `${source}: streamOptions`);
  assertKnownKeys(value.responseFormat, RESPONSE_FORMAT_KEYS, `${source}: responseFormat`);
  assertKnownKeys(value.approval, APPROVAL_KEYS, `${source}: approval`);
  for (const [i, approval] of entries(value.approvals)) {
    assertKnownKeys(approval, APPROVAL_KEYS, `${source}: approvals[${i}]`);
  }
  return value as unknown as Config;
}

// keyList returns the keys of a config type. Record<keyof T, true> makes the
// compiler reject a missing or extra key, so each list tracks its interface.
function keyList<T>(keys: Record<keyof T, true>): readonly string[] {
  return Object.keys(keys);
}

export const CONFIG_KEYS = keyList<Config>({
  operation: true, model: true, system: true, prompt: true, messages: true, uiMessages: true,
  allowSystemInMessages: true, stopWhenStepCount: true, toolChoice: true, activeTools: true,
  reasoning: true, headers: true, streamOptions: true, providerOptions: true, tools: true,
  providerTools: true, responseFormat: true, assertOutputValue: true, approval: true,
  approvals: true, expectStreamError: true, maxRetries: true,
});
export const MESSAGE_KEYS = keyList<MessageConfig>({ role: true, content: true, providerOptions: true });
export const MESSAGE_PART_KEYS = keyList<MessagePartConfig>({
  type: true, text: true, data: true, url: true, mediaType: true, filename: true, reference: true,
  toolCallId: true, toolName: true, approvalId: true, input: true, output: true, approved: true,
  reason: true, isAutomatic: true, providerExecuted: true, providerOptions: true,
});
// UI messages are upstream UIMessage values, but replay reads only these keys.
export const UI_MESSAGE_KEYS = keyList<Pick<UIMessage, "id" | "role" | "parts">>({ id: true, role: true, parts: true });
export const TOOL_KEYS = keyList<ToolConfig>({
  description: true, inputSchema: true, mockResults: true, mockError: true, modelOutput: true,
  providerOptions: true, needsApproval: true, strict: true,
});
export const TOOL_MODEL_OUTPUT_KEYS = keyList<ToolModelOutputConfig>({ type: true, text: true, content: true });
// Content items are upstream tool-result content, but replay reads only these keys.
export const TOOL_MODEL_OUTPUT_CONTENT_KEYS: readonly string[] = ["type", "text", "data", "mediaType", "filename"];
export const PROVIDER_TOOL_KEYS = keyList<ProviderToolConfig>({ id: true, args: true, inputSchema: true, providerOptions: true });
export const TOOL_CHOICE_KEYS = keyList<ToolChoiceConfig>({ type: true, toolName: true });
export const STREAM_OPTIONS_KEYS = keyList<StreamOptionsConfig>({
  sendReasoning: true, sendSources: true, sendFinish: true, sendStart: true,
});
export const RESPONSE_FORMAT_KEYS = keyList<ResponseFormatConfig>({
  type: true, outputMode: true, schema: true, choices: true, name: true, description: true,
});
export const APPROVAL_KEYS = keyList<ApprovalConfig>({
  toolCallId: true, toolName: true, approvalId: true, approved: true, reason: true, input: true,
});

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

// entries yields the items of a sequence or the named values of a mapping, and
// nothing for an absent or scalar value, which the typed builders reject later.
function entries(value: unknown): [string | number, unknown][] {
  if (Array.isArray(value)) return value.map((item, i) => [i, item]);
  if (isRecord(value)) return Object.entries(value);
  return [];
}

function assertKnownKeys(value: unknown, allowed: readonly string[], path: string): void {
  if (!isRecord(value)) return;
  const unknown = Object.keys(value).filter((key) => !allowed.includes(key));
  if (unknown.length > 0) {
    throw new Error(
      `${path}: unknown field ${unknown.map((key) => JSON.stringify(key)).join(", ")}; expected one of ${allowed.join(", ")}`,
    );
  }
}

// --- Tool Builder ---

export function buildTools(
  toolConfigs: Record<string, ToolConfig> | undefined,
  providerToolConfigs: Record<string, ProviderToolConfig> | undefined,
): Record<string, Tool> | undefined {
  if (!toolConfigs && !providerToolConfigs) return undefined;

  const tools: Record<string, Tool> = {};

  if (toolConfigs) {
    for (const [name, tc] of Object.entries(toolConfigs)) {
      let resultIdx = 0;
      const mockResults = tc.mockResults ?? [];

      const t: Tool = {
        description: tc.description,
        inputSchema: jsonSchema(tc.inputSchema),
        ...(tc.providerOptions
          ? { providerOptions: tc.providerOptions as ProviderOptions }
          : {}),
        ...(tc.needsApproval ? { needsApproval: true as const } : {}),
        ...(tc.strict != null ? { strict: tc.strict } : {}),
        ...(tc.modelOutput
          ? {
              toModelOutput: async () =>
                tc.modelOutput!.type === "text"
                  ? { type: "text" as const, value: tc.modelOutput!.text ?? "" }
                  : { type: "content" as const, value: tc.modelOutput!.content ?? [] },
            }
          : {}),
        ...(tc.mockError
          ? {
              execute: async () => {
                throw new Error(tc.mockError);
              },
            }
          : mockResults.length > 0
          ? {
              execute: async () => {
                if (resultIdx >= mockResults.length) {
                  throw new Error(
                    `No more mock results for tool "${name}" (used ${resultIdx}/${mockResults.length})`
                  );
                }
                return mockResults[resultIdx++];
              },
            }
          : {}),
      };
      tools[name] = t;
    }
  }

  if (providerToolConfigs) {
    for (const [name, ptc] of Object.entries(providerToolConfigs)) {
      tools[name] = {
        type: "provider" as const,
        id: ptc.id as `${string}.${string}`,
        args: ptc.args ?? {},
        ...(ptc.providerOptions
          ? { providerOptions: ptc.providerOptions as ProviderOptions }
          : {}),
        isProviderExecuted: true as const,
        inputSchema: ptc.inputSchema
          ? z.fromJSONSchema(ptc.inputSchema)
          : jsonSchema({ type: "object" }),
      };
    }
  }

  return tools;
}

export function buildToolChoice(cfg: Config): unknown {
  if (!cfg.toolChoice) return undefined;
  if (cfg.toolChoice.type === "tool") {
    return { type: "tool", toolName: cfg.toolChoice.toolName };
  }
  return cfg.toolChoice.type;
}

export function buildOutput(cfg: Config): unknown {
  if (!cfg.responseFormat) return undefined;

  const opts = {
    ...(cfg.responseFormat.name ? { name: cfg.responseFormat.name } : {}),
    ...(cfg.responseFormat.description ? { description: cfg.responseFormat.description } : {}),
  };

  switch (cfg.responseFormat.outputMode ?? "object") {
    case "object":
      return Output.object({
        schema: jsonSchema(cfg.responseFormat.schema),
        ...opts,
      });
    case "array":
      return Output.array({
        element: jsonSchema(cfg.responseFormat.schema),
        ...opts,
      });
    case "choice":
      if (!cfg.responseFormat.choices || cfg.responseFormat.choices.length === 0) {
        throw new Error("responseFormat.choices is required for outputMode: choice");
      }
      return Output.choice({
        options: cfg.responseFormat.choices,
        ...opts,
      });
    case "json":
      return Output.json(opts);
  }
}

export function unsupportedGenerateFields(cfg: Config): string[] {
  const nonEmptyArray = (value: unknown[] | undefined) =>
    value && value.length > 0 ? value : undefined;
  const nonEmptyObject = (value: Record<string, unknown> | undefined) =>
    value && Object.keys(value).length > 0 ? value : undefined;
  return [
    ["uiMessages", nonEmptyArray(cfg.uiMessages)],
    ["tools", nonEmptyObject(cfg.tools)],
    ["providerTools", nonEmptyObject(cfg.providerTools)],
    ["toolChoice", cfg.toolChoice],
    ["activeTools", nonEmptyArray(cfg.activeTools)],
    ["reasoning", cfg.reasoning || undefined],
    ["streamOptions", cfg.streamOptions],
    ["approval", cfg.approval],
    ["approvals", nonEmptyArray(cfg.approvals)],
    ["assertOutputValue", cfg.assertOutputValue || undefined],
    ["expectStreamError", cfg.expectStreamError || undefined],
    ["maxRetries", cfg.maxRetries],
    [
      "stopWhenStepCount",
      cfg.stopWhenStepCount != null && cfg.stopWhenStepCount > 1
        ? cfg.stopWhenStepCount
        : undefined,
    ],
  ].filter(([, value]) => value !== undefined).map(([name]) => name as string);
}

export function buildStreamTextOptions(
  cfg: Config,
  opts: StreamTextOptionsConfig,
): Record<string, unknown> {
  const toolChoice = buildToolChoice(cfg);
  return {
    model: opts.model,
    ...(opts.messages ? { messages: opts.messages } : { prompt: opts.prompt }),
    ...(cfg.system ? { instructions: cfg.system } : {}),
    ...(cfg.allowSystemInMessages ? { allowSystemInMessages: true } : {}),
    ...(opts.tools ? { tools: opts.tools } : {}),
    ...(toolChoice ? { toolChoice } : {}),
    ...(cfg.activeTools ? { activeTools: cfg.activeTools } : {}),
    ...(cfg.reasoning ? { reasoning: cfg.reasoning } : {}),
    ...(cfg.headers ? { headers: cfg.headers } : {}),
    ...(cfg.maxRetries != null ? { maxRetries: cfg.maxRetries } : {}),
    ...(opts.output ? { output: opts.output } : {}),
    stopWhen: opts.stopWhen,
    ...(cfg.providerOptions ? { providerOptions: cfg.providerOptions } : {}),
    ...(opts.generateId
      ? {
          _internal: {
            generateId: opts.generateId,
          },
        }
      : {}),
  };
}

export function buildMessages(cfg: Config, prompt: string): ModelMessage[] | undefined {
  if (cfg.messages) {
    return cfg.messages.map(buildConfiguredMessage);
  }

  const approvals = cfg.approvals ?? (cfg.approval ? [cfg.approval] : []);
  const messages: ModelMessage[] = [];
  if (approvals.length === 0) {
    return undefined;
  }
  messages.push(
    { role: "user", content: prompt },
    {
      role: "assistant",
      content: approvals.flatMap((approval) => [
        {
          type: "tool-call" as const,
          toolCallId: approval.toolCallId,
          toolName: approval.toolName,
          input: approval.input ?? {},
        },
        {
          type: "tool-approval-request" as const,
          approvalId: approval.approvalId,
          toolCallId: approval.toolCallId,
        },
      ]),
    },
    {
      role: "tool",
      content: approvals.map((approval) => ({
        type: "tool-approval-response" as const,
        approvalId: approval.approvalId,
        approved: approval.approved,
        ...(approval.reason ? { reason: approval.reason } : {}),
      })),
    },
  );
  return messages;
}

function buildConfiguredMessage(message: MessageConfig): ModelMessage {
  const providerOptions = message.providerOptions
    ? { providerOptions: message.providerOptions as ProviderOptions }
    : {};
  const content = Array.isArray(message.content)
    ? message.content.map(buildConfiguredPart)
    : message.content;

  return {
    role: message.role,
    content,
    ...providerOptions,
  } as ModelMessage;
}

function buildConfiguredPart(part: MessagePartConfig) {
  switch (part.type) {
    case "text":
      return {
        type: "text" as const,
        text: part.text ?? "",
        ...(part.providerOptions
          ? { providerOptions: part.providerOptions as ProviderOptions }
          : {}),
      };
    case "reasoning":
      return {
        type: "reasoning" as const,
        text: part.text ?? "",
        ...(part.providerOptions
          ? { providerOptions: part.providerOptions as ProviderOptions }
          : {}),
      };
    case "file":
      return {
        type: "file" as const,
        data: part.url
          ? new URL(part.url)
          : part.reference
            ? { type: "reference" as const, reference: part.reference }
            : (part.data ?? ""),
        mediaType: part.mediaType,
        ...(part.filename ? { filename: part.filename } : {}),
        ...(part.providerOptions
          ? { providerOptions: part.providerOptions as ProviderOptions }
          : {}),
      };
    case "tool-call":
      return {
        type: "tool-call" as const,
        toolCallId: part.toolCallId ?? "",
        toolName: part.toolName ?? "",
        input: part.input,
        ...(part.providerExecuted ? { providerExecuted: true } : {}),
        ...(part.providerOptions
          ? { providerOptions: part.providerOptions as ProviderOptions }
          : {}),
      };
    case "tool-result":
      return {
        type: "tool-result" as const,
        toolCallId: part.toolCallId ?? "",
        toolName: part.toolName ?? "",
        output: part.output,
        ...(part.providerOptions
          ? { providerOptions: part.providerOptions as ProviderOptions }
          : {}),
      };
    case "tool-approval-request":
      return {
        type: "tool-approval-request" as const,
        approvalId: part.approvalId ?? "",
        toolCallId: part.toolCallId ?? "",
        ...(part.isAutomatic ? { isAutomatic: true } : {}),
        ...(part.providerOptions
          ? { providerOptions: part.providerOptions as ProviderOptions }
          : {}),
      };
    case "tool-approval-response":
      return {
        type: "tool-approval-response" as const,
        approvalId: part.approvalId ?? "",
        approved: part.approved ?? false,
        ...(part.reason ? { reason: part.reason } : {}),
        ...(part.providerExecuted ? { providerExecuted: true } : {}),
        ...(part.providerOptions
          ? { providerOptions: part.providerOptions as ProviderOptions }
          : {}),
      };
  }
}

// --- Request Snapshots ---

const redactedHeaderValue = "<redacted>";

const secretHeaderNames = new Set([
  "authorization",
  "x-api-key",
  "api-key",
  "anthropic-api-key",
]);

const providerHeaderAllowlists: Record<string, Set<string>> = {
  anthropic: new Set([
    "content-type",
    "anthropic-version",
    "anthropic-beta",
    "authorization",
    "x-api-key",
  ]),
  openai: new Set([
    "content-type",
    "authorization",
    "openai-beta",
    "openai-organization",
    "openai-project",
    "x-ai-sdk-test",
  ]),
  // Bedrock authenticates via SigV4, so authorization, x-amz-*, host,
  // user-agent, and content-length are all volatile and intentionally
  // excluded. "accept" is also excluded: upstream relies on the JS fetch
  // default ("*/*") which carries no behavioral signal, while the Go client
  // sends none. Only content-type is asserted; the request body carries the
  // meaningful conformance signal.
  bedrock: new Set(["content-type"]),
  "openai-compatible": new Set(["authorization", "content-type"]),
};

export function normalizeRequestSnapshot(
  provider: string,
  req: { method?: string; url?: string; headers: IncomingHttpHeaders },
  bodyText: string,
): RequestSnapshot {
  return {
    method: (req.method ?? "").toUpperCase(),
    path: normalizeRequestPath(req.url),
    headers: normalizeRequestHeaders(provider, req.headers),
    body: parseRequestBody(bodyText),
  };
}

export function writeRequestSnapshots(path: string, snapshots: RequestSnapshot[]): void {
  writeFileSync(path, snapshots.map(stableStringify).join("\n") + (snapshots.length > 0 ? "\n" : ""));
}

function normalizeRequestPath(rawURL: string | undefined): string {
  const target = (rawURL ?? "/").split("#", 1)[0];
  const url = new URL(target, "http://localhost");
  const queryIndex = target.indexOf("?");
  const query = queryIndex < 0 || queryIndex === target.length - 1 ? "" : target.slice(queryIndex);
  return url.pathname + query;
}

function normalizeRequestHeaders(
  provider: string,
  headers: IncomingHttpHeaders,
): Record<string, string> {
  const allowlist = providerHeaderAllowlists[provider] ?? new Set<string>();
  const result: Record<string, string> = {};

  for (const [rawName, rawValue] of Object.entries(headers)) {
    const name = rawName.toLowerCase();
    if (!allowlist.has(name)) continue;

    let value = headerValueToString(rawValue);
    if (value === undefined) continue;
    if (name === "anthropic-beta") value = normalizeBetaHeader(value);

    result[name] = secretHeaderNames.has(name) ? redactedHeaderValue : value;
  }

  return sortObject(result) as Record<string, string>;
}

function headerValueToString(value: string | string[] | undefined): string | undefined {
  if (value === undefined) return undefined;
  const normalized = Array.isArray(value) ? value.join(", ") : value;
  const trimmed = normalized.trim();
  return trimmed === "" ? undefined : trimmed;
}

function normalizeBetaHeader(value: string): string {
  return value
    .split(",")
    .map((part) => part.trim())
    .filter(Boolean)
    .sort((a, b) => a.localeCompare(b))
    .join(",");
}

function parseRequestBody(bodyText: string): unknown {
  const trimmed = bodyText.trim();
  if (trimmed === "") return null;
  return JSON.parse(trimmed);
}

function stableStringify(value: unknown): string {
  return JSON.stringify(stableJSONValue(value));
}

function stableJSONValue(value: unknown): unknown {
  return stableJSONValueForKey("", value);
}

function stableJSONValueForKey(key: string, value: unknown): unknown {
  if (Array.isArray(value)) {
    const normalized = value.map((item) => stableJSONValueForKey("", item));
    if (key === "tools") {
      return normalized.sort((a, b) => toolSortKey(a).localeCompare(toolSortKey(b)));
    }
    return normalized;
  }
  if (value !== null && typeof value === "object") {
    const record = value as Record<string, unknown>;
    if (record.type === "tool_result" && "content" in record) {
      record.content = normalizeToolResultContent(record.content);
    }
    // OpenAI function_call_output carries the tool result as a JSON string;
    // parse it so object field ordering is compared insensitively, matching the
    // upstream serialization which preserves tool-result key order.
    if (record.type === "function_call_output" && typeof record.output === "string") {
      record.output = parseJSONIfPossible(record.output);
    }
    if (record.type === "web_search_result" && record.page_age == null) {
      delete record.page_age;
    }
    return sortObject(record);
  }
  return value;
}

function normalizeToolResultContent(content: unknown): unknown {
  if (typeof content === "string") {
    return parseJSONIfPossible(content);
  }
  if (Array.isArray(content) && content.length === 1) {
    const first = content[0];
    if (first !== null && typeof first === "object") {
      const block = first as Record<string, unknown>;
      if (block.type === "text" && typeof block.text === "string") {
        return parseJSONIfPossible(block.text);
      }
    }
  }
  return content;
}

function parseJSONIfPossible(value: string): unknown {
  try {
    return JSON.parse(value);
  } catch {
    return value;
  }
}

function sortObject(obj: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(obj)
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([key, value]) => [key, stableJSONValueForKey(key, value)]),
  );
}

function toolSortKey(value: unknown): string {
  if (value !== null && typeof value === "object") {
    const tool = value as Record<string, unknown>;
    if (tool.toolSpec !== null && typeof tool.toolSpec === "object") {
      const spec = tool.toolSpec as Record<string, unknown>;
      return `${String(spec.name ?? "")}\u0000toolSpec`;
    }
    return `${String(tool.name ?? "")}\u0000${String(tool.type ?? "")}`;
  }
  return "";
}
