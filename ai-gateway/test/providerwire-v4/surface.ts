import type {
  LanguageModelV4CallOptions,
  LanguageModelV4Content,
  LanguageModelV4File,
  LanguageModelV4FinishReason,
  LanguageModelV4Message,
  LanguageModelV4StreamPart,
  LanguageModelV4ToolApprovalResponsePart,
  LanguageModelV4ToolChoice,
  LanguageModelV4ToolResultOutput,
  SharedV4FileData,
  SharedV4Warning,
} from "@ai-sdk/provider";

type Witness<T extends PropertyKey> = Record<T, true>;
type MessageFor<Role extends LanguageModelV4Message["role"]> = Extract<
  LanguageModelV4Message,
  { role: Role }
>;
type ContentType<Role extends Exclude<LanguageModelV4Message["role"], "system">> =
  MessageFor<Role>["content"][number]["type"];
type ToolResultContent = Extract<LanguageModelV4ToolResultOutput, { type: "content" }>[
  "value"
][number];
type AssistantReasoningFile = Extract<
  MessageFor<"assistant">["content"][number],
  { type: "reasoning-file" }
>;
type CallOptionTool = NonNullable<LanguageModelV4CallOptions["tools"]>[number];

type SerializedCallOptionKey = keyof Omit<LanguageModelV4CallOptions, "abortSignal">;

export const callOptionKeys = {
  prompt: true,
  maxOutputTokens: true,
  temperature: true,
  stopSequences: true,
  topP: true,
  topK: true,
  presencePenalty: true,
  frequencyPenalty: true,
  responseFormat: true,
  seed: true,
  tools: true,
  toolChoice: true,
  includeRawChunks: true,
  headers: true,
  reasoning: true,
  providerOptions: true,
} as const satisfies Witness<SerializedCallOptionKey>;

export const messageRoles = {
  system: true,
  user: true,
  assistant: true,
  tool: true,
} as const satisfies Witness<LanguageModelV4Message["role"]>;

export const userContentTypes = {
  text: true,
  file: true,
} as const satisfies Witness<ContentType<"user">>;

export const assistantContentTypes = {
  text: true,
  file: true,
  custom: true,
  reasoning: true,
  "reasoning-file": true,
  "tool-call": true,
  "tool-result": true,
} as const satisfies Witness<ContentType<"assistant">>;

export const toolContentTypes = {
  "tool-result": true,
  "tool-approval-response": true,
} as const satisfies Witness<ContentType<"tool">>;

export const approvalResponseTypes = {
  "tool-approval-response": true,
} as const satisfies Witness<LanguageModelV4ToolApprovalResponsePart["type"]>;

export const fileDataTypes = {
  data: true,
  url: true,
  reference: true,
  text: true,
} as const satisfies Witness<SharedV4FileData["type"]>;

export const reasoningFileDataTypes = {
  data: true,
  url: true,
} as const satisfies Witness<AssistantReasoningFile["data"]["type"]>;

export const generatedFileDataTypes = {
  data: true,
  url: true,
} as const satisfies Witness<LanguageModelV4File["data"]["type"]>;

export const toolResultOutputTypes = {
  text: true,
  json: true,
  "execution-denied": true,
  "error-text": true,
  "error-json": true,
  content: true,
} as const satisfies Witness<LanguageModelV4ToolResultOutput["type"]>;

export const toolResultContentTypes = {
  text: true,
  file: true,
  custom: true,
} as const satisfies Witness<ToolResultContent["type"]>;

export const toolTypes = {
  function: true,
  provider: true,
} as const satisfies Witness<CallOptionTool["type"]>;

export const toolChoiceTypes = {
  auto: true,
  none: true,
  required: true,
  tool: true,
} as const satisfies Witness<LanguageModelV4ToolChoice["type"]>;

export const responseFormatTypes = {
  text: true,
  json: true,
} as const satisfies Witness<
  NonNullable<LanguageModelV4CallOptions["responseFormat"]>["type"]
>;

export const reasoningValues = {
  "provider-default": true,
  none: true,
  minimal: true,
  low: true,
  medium: true,
  high: true,
  xhigh: true,
} as const satisfies Witness<NonNullable<LanguageModelV4CallOptions["reasoning"]>>;

export const generatedContentTypes = {
  text: true,
  reasoning: true,
  custom: true,
  "reasoning-file": true,
  file: true,
  "tool-approval-request": true,
  source: true,
  "tool-call": true,
  "tool-result": true,
} as const satisfies Witness<LanguageModelV4Content["type"]>;

export const sourceTypes = {
  url: true,
  document: true,
} as const satisfies Witness<
  Extract<LanguageModelV4Content, { type: "source" }>["sourceType"]
>;

export const streamPartTypes = {
  "text-start": true,
  "text-delta": true,
  "text-end": true,
  "reasoning-start": true,
  "reasoning-delta": true,
  "reasoning-end": true,
  "tool-input-start": true,
  "tool-input-delta": true,
  "tool-input-end": true,
  "tool-approval-request": true,
  "tool-call": true,
  "tool-result": true,
  custom: true,
  file: true,
  "reasoning-file": true,
  source: true,
  "stream-start": true,
  "response-metadata": true,
  finish: true,
  raw: true,
  error: true,
} as const satisfies Witness<LanguageModelV4StreamPart["type"]>;

export const warningTypes = {
  unsupported: true,
  compatibility: true,
  deprecated: true,
  other: true,
} as const satisfies Witness<SharedV4Warning["type"]>;

export const finishReasons = {
  stop: true,
  length: true,
  "content-filter": true,
  "tool-calls": true,
  error: true,
  other: true,
} as const satisfies Witness<LanguageModelV4FinishReason["unified"]>;
