import type {
  LanguageModelV4CallOptions,
  LanguageModelV4CustomPart,
  LanguageModelV4FilePart,
  LanguageModelV4FunctionTool,
  LanguageModelV4Prompt,
  LanguageModelV4ProviderTool,
  LanguageModelV4ReasoningFilePart,
  LanguageModelV4ReasoningPart,
  LanguageModelV4TextPart,
  LanguageModelV4ToolApprovalResponsePart,
  LanguageModelV4ToolCallPart,
  LanguageModelV4ToolChoice,
  LanguageModelV4ToolResultOutput,
  LanguageModelV4ToolResultPart,
  SharedV4FileData,
} from "@ai-sdk/provider";

type Message = LanguageModelV4Prompt[number];
type SystemMessage = Extract<Message, { role: "system" }>;
type UserMessage = Extract<Message, { role: "user" }>;
type AssistantMessage = Extract<Message, { role: "assistant" }>;
type ToolMessage = Extract<Message, { role: "tool" }>;
type ArrayContent = Extract<Message["content"], unknown[]>;
type PromptPart = ArrayContent[number];
type UserContent = UserMessage["content"][number];
type AssistantContent = AssistantMessage["content"][number];
type ToolContent = ToolMessage["content"][number];
type ReasoningFileData = LanguageModelV4ReasoningFilePart["data"];
type Tool = NonNullable<LanguageModelV4CallOptions["tools"]>[number];
type ResponseFormat = NonNullable<LanguageModelV4CallOptions["responseFormat"]>;
type ResponseFormatText = Extract<ResponseFormat, { type: "text" }>;
type ResponseFormatJSON = Extract<ResponseFormat, { type: "json" }>;
type Reasoning = NonNullable<LanguageModelV4CallOptions["reasoning"]>;
type InputExample = NonNullable<LanguageModelV4FunctionTool["inputExamples"]>[number];
type AutoToolChoice = Extract<LanguageModelV4ToolChoice, { type: "auto" }>;
type NoneToolChoice = Extract<LanguageModelV4ToolChoice, { type: "none" }>;
type RequiredToolChoice = Extract<LanguageModelV4ToolChoice, { type: "required" }>;
type ForcedToolChoice = Extract<LanguageModelV4ToolChoice, { type: "tool" }>;
type FileDataData = Extract<SharedV4FileData, { type: "data" }>;
type FileDataURL = Extract<SharedV4FileData, { type: "url" }>;
type FileDataReference = Extract<SharedV4FileData, { type: "reference" }>;
type FileDataText = Extract<SharedV4FileData, { type: "text" }>;
type ToolOutputText = Extract<LanguageModelV4ToolResultOutput, { type: "text" }>;
type ToolOutputJSON = Extract<LanguageModelV4ToolResultOutput, { type: "json" }>;
type ToolOutputDenied = Extract<LanguageModelV4ToolResultOutput, { type: "execution-denied" }>;
type ToolOutputErrorText = Extract<LanguageModelV4ToolResultOutput, { type: "error-text" }>;
type ToolOutputErrorJSON = Extract<LanguageModelV4ToolResultOutput, { type: "error-json" }>;
type ToolOutputContent = Extract<LanguageModelV4ToolResultOutput, { type: "content" }>;
type ToolResultContentPart = ToolOutputContent["value"][number];
type ToolOutputContentText = Extract<ToolResultContentPart, { type: "text" }>;
type ToolOutputContentFile = Extract<ToolResultContentPart, { type: "file" }>;
type ToolOutputContentCustom = Extract<ToolResultContentPart, { type: "custom" }>;

type ScenarioName =
  | "unary-settings"
  | "response-format-text"
  | "reasoning-provider-default"
  | "reasoning-none"
  | "reasoning-minimal"
  | "reasoning-low"
  | "reasoning-medium"
  | "reasoning-high"
  | "reasoning-xhigh"
  | "streaming-prompt-tools"
  | "presence-losses"
  | "tool-choice-auto"
  | "tool-choice-none"
  | "tool-choice-required"
  | "tool-choice-tool"
  | "header-call"
  | "header-exact-custom"
  | "header-case-custom"
  | "header-exact-protocol"
  | "header-case-protocol"
  | "header-exact-content-type"
  | "header-case-content-type"
  | "header-observability"
  | "multi-step-tool"
;

type GatewayTransform =
  | "abort-signal-excluded"
  | "uint8array-base64"
  | "language-model-relative-url"
  | "call-header-body"
  | "call-header-outer"
  | "same-key-replacement"
  | "case-variant-last-value"
  | "exact-protocol-replacement"
  | "case-variant-protocol-last-value"
  | "exact-content-type-exclusion"
  | "case-variant-content-type-exclusion"
  | "observability-last-value"
  | "call-trace-body"
  | "call-trace-outer"
  | "observability-body";

export type CoverageEntry =
  | { scenario: ScenarioName; path: `/${string}`; presence: "present" }
  | { scenario: ScenarioName; path: `/${string}`; expected: unknown }
  | { exclusion: string };

export interface CoverageCategory {
  id: string;
  source: string;
  members: Record<string, CoverageEntry>;
}

export const requestCoverage = [
  {
    id: "call-options",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-call-options.ts",
    members: {
      "abortSignal": {"exclusion":"Gateway getArgs removes abortSignal before body serialization and uses it only for Fetch cancellation."},
      "frequencyPenalty": {"expected":0,"path":"/requests/0/body/frequencyPenalty","scenario":"unary-settings"},
      "headers": {"expected":{},"path":"/requests/0/body/headers","scenario":"unary-settings"},
      "includeRawChunks": {"expected":false,"path":"/requests/0/body/includeRawChunks","scenario":"unary-settings"},
      "maxOutputTokens": {"expected":0,"path":"/requests/0/body/maxOutputTokens","scenario":"unary-settings"},
      "presencePenalty": {"expected":0,"path":"/requests/0/body/presencePenalty","scenario":"unary-settings"},
      "prompt": {"scenario":"unary-settings","path":"/requests/0/body/prompt","expected":[]},
      "providerOptions": {"expected":{},"path":"/requests/0/body/providerOptions","scenario":"presence-losses"},
      "reasoning": {"path":"/requests/0/body/reasoning","presence":"present","scenario":"unary-settings"},
      "responseFormat": {"path":"/requests/0/body/responseFormat","presence":"present","scenario":"unary-settings"},
      "seed": {"expected":0,"path":"/requests/0/body/seed","scenario":"unary-settings"},
      "stopSequences": {"expected":[],"path":"/requests/0/body/stopSequences","scenario":"unary-settings"},
      "temperature": {"expected":0,"path":"/requests/0/body/temperature","scenario":"unary-settings"},
      "toolChoice": {"path":"/requests/0/body/toolChoice","presence":"present","scenario":"tool-choice-auto"},
      "tools": {"scenario":"unary-settings","path":"/requests/0/body/tools","expected":[]},
      "topK": {"expected":0,"path":"/requests/0/body/topK","scenario":"unary-settings"},
      "topP": {"expected":0,"path":"/requests/0/body/topP","scenario":"unary-settings"},
    } satisfies Record<keyof LanguageModelV4CallOptions, CoverageEntry>,
  },
  {
    id: "message-role",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "assistant": {"expected":"assistant","path":"/requests/0/body/prompt/2/role","scenario":"streaming-prompt-tools"},
      "system": {"expected":"system","path":"/requests/0/body/prompt/0/role","scenario":"streaming-prompt-tools"},
      "tool": {"expected":"tool","path":"/requests/0/body/prompt/3/role","scenario":"streaming-prompt-tools"},
      "user": {"expected":"user","path":"/requests/0/body/prompt/1/role","scenario":"streaming-prompt-tools"},
    } satisfies Record<Message["role"], CoverageEntry>,
  },
  {
    id: "file-data",
    source: "@ai-sdk/provider/src/shared/v4/shared-v4-file-data.ts",
    members: {
      "data": {"expected":"data","path":"/requests/0/body/prompt/1/content/1/data/type","scenario":"streaming-prompt-tools"},
      "reference": {"expected":"reference","path":"/requests/0/body/prompt/1/content/3/data/type","scenario":"streaming-prompt-tools"},
      "text": {"expected":"text","path":"/requests/0/body/prompt/1/content/4/data/type","scenario":"streaming-prompt-tools"},
      "url": {"expected":"url","path":"/requests/0/body/prompt/1/content/2/data/type","scenario":"streaming-prompt-tools"},
    } satisfies Record<SharedV4FileData["type"], CoverageEntry>,
  },
  {
    id: "tool",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-function-tool.ts and language-model-v4-provider-tool.ts",
    members: {
      "function": {"expected":"function","path":"/requests/0/body/tools/0/type","scenario":"streaming-prompt-tools"},
      "provider": {"expected":"provider","path":"/requests/0/body/tools/1/type","scenario":"streaming-prompt-tools"},
    } satisfies Record<Tool["type"], CoverageEntry>,
  },
  {
    id: "tool-choice",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-tool-choice.ts",
    members: {
      "auto": {"expected":"auto","path":"/requests/0/body/toolChoice/type","scenario":"tool-choice-auto"},
      "none": {"expected":"none","path":"/requests/0/body/toolChoice/type","scenario":"tool-choice-none"},
      "required": {"expected":"required","path":"/requests/0/body/toolChoice/type","scenario":"tool-choice-required"},
      "tool": {"expected":"tool","path":"/requests/0/body/toolChoice/type","scenario":"tool-choice-tool"},
    } satisfies Record<LanguageModelV4ToolChoice["type"], CoverageEntry>,
  },
  {
    id: "response-format",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-call-options.ts",
    members: {
      "json": {"expected":"json","path":"/requests/0/body/responseFormat/type","scenario":"unary-settings"},
      "text": {"expected":"text","path":"/requests/0/body/responseFormat/type","scenario":"response-format-text"},
    } satisfies Record<ResponseFormat["type"], CoverageEntry>,
  },
  {
    id: "reasoning",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-call-options.ts",
    members: {
      "high": {"expected":"high","path":"/requests/0/body/reasoning","scenario":"reasoning-high"},
      "low": {"expected":"low","path":"/requests/0/body/reasoning","scenario":"reasoning-low"},
      "medium": {"expected":"medium","path":"/requests/0/body/reasoning","scenario":"reasoning-medium"},
      "minimal": {"expected":"minimal","path":"/requests/0/body/reasoning","scenario":"reasoning-minimal"},
      "none": {"expected":"none","path":"/requests/0/body/reasoning","scenario":"reasoning-none"},
      "provider-default": {"expected":"provider-default","path":"/requests/0/body/reasoning","scenario":"reasoning-provider-default"},
      "xhigh": {"expected":"xhigh","path":"/requests/0/body/reasoning","scenario":"reasoning-xhigh"},
    } satisfies Record<Reasoning, CoverageEntry>,
  },
  {
    id: "tool-result-output",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "content": {"expected":"content","path":"/requests/0/body/prompt/2/content/12/output/type","scenario":"streaming-prompt-tools"},
      "error-json": {"expected":"error-json","path":"/requests/0/body/prompt/2/content/11/output/type","scenario":"streaming-prompt-tools"},
      "error-text": {"expected":"error-text","path":"/requests/0/body/prompt/2/content/10/output/type","scenario":"streaming-prompt-tools"},
      "execution-denied": {"expected":"execution-denied","path":"/requests/0/body/prompt/2/content/9/output/type","scenario":"streaming-prompt-tools"},
      "json": {"expected":"json","path":"/requests/0/body/prompt/2/content/8/output/type","scenario":"streaming-prompt-tools"},
      "text": {"expected":"text","path":"/requests/0/body/prompt/2/content/7/output/type","scenario":"streaming-prompt-tools"},
    } satisfies Record<LanguageModelV4ToolResultOutput["type"], CoverageEntry>,
  },
  {
    id: "tool-result-content",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "custom": {"expected":"custom","path":"/requests/0/body/prompt/2/content/12/output/value/2/type","scenario":"streaming-prompt-tools"},
      "file": {"expected":"file","path":"/requests/0/body/prompt/2/content/12/output/value/1/type","scenario":"streaming-prompt-tools"},
      "text": {"expected":"text","path":"/requests/0/body/prompt/2/content/12/output/value/0/type","scenario":"streaming-prompt-tools"},
    } satisfies Record<ToolResultContentPart["type"], CoverageEntry>,
  },
  {
    id: "user-content",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "text": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/1/content/0/type","expected":"text"},
      "file": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/1/content/1/type","expected":"file"},
    } satisfies Record<UserContent["type"], CoverageEntry>,
  },
  {
    id: "assistant-content",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "text": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/0/type","expected":"text"},
      "file": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/1/type","expected":"file"},
      "custom": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/2/type","expected":"custom"},
      "reasoning": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/3/type","expected":"reasoning"},
      "reasoning-file": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/4/type","expected":"reasoning-file"},
      "tool-call": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/6/type","expected":"tool-call"},
      "tool-result": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/7/type","expected":"tool-result"},
    } satisfies Record<AssistantContent["type"], CoverageEntry>,
  },
  {
    id: "tool-content",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "tool-result": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/3/content/0/type","expected":"tool-result"},
      "tool-approval-response": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/3/content/1/type","expected":"tool-approval-response"},
    } satisfies Record<ToolContent["type"], CoverageEntry>,
  },
  {
    id: "reasoning-file-data",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "data": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/4/data/type","expected":"data"},
      "url": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/5/data/type","expected":"url"},
    } satisfies Record<ReasoningFileData["type"], CoverageEntry>,
  },
  {
    id: "system-message",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "role": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/0/role","expected":"system"},
      "content": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/0/content","presence":"present"},
      "providerOptions": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/0/providerOptions","presence":"present"},
    } satisfies Record<keyof SystemMessage, CoverageEntry>,
  },
  {
    id: "user-message",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "role": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/1/role","expected":"user"},
      "content": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/1/content","presence":"present"},
      "providerOptions": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/1/providerOptions","presence":"present"},
    } satisfies Record<keyof UserMessage, CoverageEntry>,
  },
  {
    id: "assistant-message",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "role": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/role","expected":"assistant"},
      "content": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content","presence":"present"},
      "providerOptions": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/providerOptions","presence":"present"},
    } satisfies Record<keyof AssistantMessage, CoverageEntry>,
  },
  {
    id: "tool-message",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "role": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/3/role","expected":"tool"},
      "content": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/3/content","presence":"present"},
      "providerOptions": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/3/providerOptions","presence":"present"},
    } satisfies Record<keyof ToolMessage, CoverageEntry>,
  },
  {
    id: "text-part",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"presence-losses","path":"/requests/0/body/prompt/1/content/0/type","expected":"text"},
      "text": {"scenario":"presence-losses","path":"/requests/0/body/prompt/1/content/0/text","expected":""},
      "providerOptions": {"scenario":"presence-losses","path":"/requests/0/body/prompt/1/content/0/providerOptions","expected":{}},
    } satisfies Record<keyof LanguageModelV4TextPart, CoverageEntry>,
  },
  {
    id: "file-part",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"presence-losses","path":"/requests/0/body/prompt/1/content/1/type","expected":"file"},
      "filename": {"scenario":"presence-losses","path":"/requests/0/body/prompt/1/content/1/filename","expected":""},
      "data": {"scenario":"presence-losses","path":"/requests/0/body/prompt/1/content/1/data","presence":"present"},
      "mediaType": {"scenario":"presence-losses","path":"/requests/0/body/prompt/1/content/1/mediaType","expected":""},
      "providerOptions": {"scenario":"presence-losses","path":"/requests/0/body/prompt/1/content/1/providerOptions","expected":{}},
    } satisfies Record<keyof LanguageModelV4FilePart, CoverageEntry>,
  },
  {
    id: "custom-part",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/0/type","expected":"custom"},
      "kind": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/0/kind","presence":"present"},
      "providerOptions": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/0/providerOptions","expected":{}},
    } satisfies Record<keyof LanguageModelV4CustomPart, CoverageEntry>,
  },
  {
    id: "reasoning-part",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/3/type","expected":"reasoning"},
      "text": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/3/text","presence":"present"},
      "providerOptions": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/3/providerOptions","presence":"present"},
    } satisfies Record<keyof LanguageModelV4ReasoningPart, CoverageEntry>,
  },
  {
    id: "reasoning-file-part",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/4/type","expected":"reasoning-file"},
      "data": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/4/data","presence":"present"},
      "mediaType": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/4/mediaType","presence":"present"},
      "providerOptions": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/4/providerOptions","presence":"present"},
    } satisfies Record<keyof LanguageModelV4ReasoningFilePart, CoverageEntry>,
  },
  {
    id: "tool-call-part",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/1/type","expected":"tool-call"},
      "toolCallId": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/1/toolCallId","expected":""},
      "toolName": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/1/toolName","expected":""},
      "input": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/1/input","expected":{}},
      "providerExecuted": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/1/providerExecuted","expected":false},
      "providerOptions": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/1/providerOptions","expected":{}},
    } satisfies Record<keyof LanguageModelV4ToolCallPart, CoverageEntry>,
  },
  {
    id: "tool-result-part",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/2/type","expected":"tool-result"},
      "toolCallId": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/2/toolCallId","expected":""},
      "toolName": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/2/toolName","expected":""},
      "output": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/2/output","presence":"present"},
      "providerOptions": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/2/providerOptions","expected":{}},
    } satisfies Record<keyof LanguageModelV4ToolResultPart, CoverageEntry>,
  },
  {
    id: "approval-response-part",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"presence-losses","path":"/requests/0/body/prompt/3/content/0/type","expected":"tool-approval-response"},
      "approvalId": {"scenario":"presence-losses","path":"/requests/0/body/prompt/3/content/0/approvalId","expected":""},
      "approved": {"scenario":"presence-losses","path":"/requests/0/body/prompt/3/content/0/approved","expected":false},
      "reason": {"scenario":"presence-losses","path":"/requests/0/body/prompt/3/content/0/reason","expected":""},
      "providerOptions": {"scenario":"presence-losses","path":"/requests/0/body/prompt/3/content/0/providerOptions","expected":{}},
    } satisfies Record<keyof LanguageModelV4ToolApprovalResponsePart, CoverageEntry>,
  },
  {
    id: "file-data-data",
    source: "@ai-sdk/provider/src/shared/v4/shared-v4-file-data.ts",
    members: {
      "type": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/1/content/1/data/type","expected":"data"},
      "data": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/1/content/1/data/data","presence":"present"},
    } satisfies Record<keyof FileDataData, CoverageEntry>,
  },
  {
    id: "file-data-url",
    source: "@ai-sdk/provider/src/shared/v4/shared-v4-file-data.ts",
    members: {
      "type": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/1/content/2/data/type","expected":"url"},
      "url": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/1/content/2/data/url","presence":"present"},
    } satisfies Record<keyof FileDataURL, CoverageEntry>,
  },
  {
    id: "file-data-reference",
    source: "@ai-sdk/provider/src/shared/v4/shared-v4-file-data.ts",
    members: {
      "type": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/1/content/3/data/type","expected":"reference"},
      "reference": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/1/content/3/data/reference","presence":"present"},
    } satisfies Record<keyof FileDataReference, CoverageEntry>,
  },
  {
    id: "file-data-text",
    source: "@ai-sdk/provider/src/shared/v4/shared-v4-file-data.ts",
    members: {
      "type": {"scenario":"presence-losses","path":"/requests/0/body/prompt/1/content/1/data/type","expected":"text"},
      "text": {"scenario":"presence-losses","path":"/requests/0/body/prompt/1/content/1/data/text","expected":""},
    } satisfies Record<keyof FileDataText, CoverageEntry>,
  },
  {
    id: "function-tool",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-function-tool.ts and language-model-v4-provider-tool.ts",
    members: {
      "type": {"scenario":"presence-losses","path":"/requests/0/body/tools/0/type","expected":"function"},
      "name": {"scenario":"presence-losses","path":"/requests/0/body/tools/0/name","expected":""},
      "description": {"scenario":"presence-losses","path":"/requests/0/body/tools/0/description","expected":""},
      "inputSchema": {"scenario":"presence-losses","path":"/requests/0/body/tools/0/inputSchema","presence":"present"},
      "inputExamples": {"scenario":"presence-losses","path":"/requests/0/body/tools/0/inputExamples","expected":[]},
      "strict": {"scenario":"presence-losses","path":"/requests/0/body/tools/0/strict","expected":false},
      "providerOptions": {"scenario":"presence-losses","path":"/requests/0/body/tools/0/providerOptions","expected":{}},
    } satisfies Record<keyof LanguageModelV4FunctionTool, CoverageEntry>,
  },
  {
    id: "provider-tool",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-function-tool.ts and language-model-v4-provider-tool.ts",
    members: {
      "type": {"scenario":"presence-losses","path":"/requests/0/body/tools/1/type","expected":"provider"},
      "id": {"scenario":"presence-losses","path":"/requests/0/body/tools/1/id","presence":"present"},
      "name": {"scenario":"presence-losses","path":"/requests/0/body/tools/1/name","expected":""},
      "args": {"scenario":"presence-losses","path":"/requests/0/body/tools/1/args","expected":{}},
    } satisfies Record<keyof LanguageModelV4ProviderTool, CoverageEntry>,
  },
  {
    id: "input-example",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-function-tool.ts",
    members: {
      "input": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/tools/0/inputExamples/0/input","presence":"present"},
    } satisfies Record<keyof InputExample, CoverageEntry>,
  },
  {
    id: "tool-choice-auto-arm",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-tool-choice.ts",
    members: {
      "type": {"scenario":"tool-choice-auto","path":"/requests/0/body/toolChoice/type","expected":"auto"},
    } satisfies Record<keyof AutoToolChoice, CoverageEntry>,
  },
  {
    id: "tool-choice-none-arm",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-tool-choice.ts",
    members: {
      "type": {"scenario":"tool-choice-none","path":"/requests/0/body/toolChoice/type","expected":"none"},
    } satisfies Record<keyof NoneToolChoice, CoverageEntry>,
  },
  {
    id: "tool-choice-required-arm",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-tool-choice.ts",
    members: {
      "type": {"scenario":"tool-choice-required","path":"/requests/0/body/toolChoice/type","expected":"required"},
    } satisfies Record<keyof RequiredToolChoice, CoverageEntry>,
  },
  {
    id: "tool-choice-tool-arm",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-tool-choice.ts",
    members: {
      "type": {"scenario":"presence-losses","path":"/requests/0/body/toolChoice/type","expected":"tool"},
      "toolName": {"scenario":"presence-losses","path":"/requests/0/body/toolChoice/toolName","expected":""},
    } satisfies Record<keyof ForcedToolChoice, CoverageEntry>,
  },
  {
    id: "response-format-text-arm",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-call-options.ts",
    members: {
      "type": {"scenario":"response-format-text","path":"/requests/0/body/responseFormat/type","expected":"text"},
    } satisfies Record<keyof ResponseFormatText, CoverageEntry>,
  },
  {
    id: "response-format-json-arm",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-call-options.ts",
    members: {
      "type": {"scenario":"presence-losses","path":"/requests/0/body/responseFormat/type","expected":"json"},
      "schema": {"scenario":"presence-losses","path":"/requests/0/body/responseFormat/schema","presence":"present"},
      "name": {"scenario":"presence-losses","path":"/requests/0/body/responseFormat/name","expected":""},
      "description": {"scenario":"presence-losses","path":"/requests/0/body/responseFormat/description","expected":""},
    } satisfies Record<keyof ResponseFormatJSON, CoverageEntry>,
  },
  {
    id: "tool-output-text",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/2/output/type","expected":"text"},
      "value": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/2/output/value","expected":""},
      "providerOptions": {"scenario":"presence-losses","path":"/requests/0/body/prompt/2/content/2/output/providerOptions","expected":{}},
    } satisfies Record<keyof ToolOutputText, CoverageEntry>,
  },
  {
    id: "tool-output-json",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/8/output/type","expected":"json"},
      "value": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/8/output/value","presence":"present"},
      "providerOptions": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/8/output/providerOptions","presence":"present"},
    } satisfies Record<keyof ToolOutputJSON, CoverageEntry>,
  },
  {
    id: "tool-output-denied",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/9/output/type","expected":"execution-denied"},
      "reason": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/9/output/reason","expected":""},
      "providerOptions": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/9/output/providerOptions","presence":"present"},
    } satisfies Record<keyof ToolOutputDenied, CoverageEntry>,
  },
  {
    id: "tool-output-error-text",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/10/output/type","expected":"error-text"},
      "value": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/10/output/value","presence":"present"},
      "providerOptions": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/10/output/providerOptions","presence":"present"},
    } satisfies Record<keyof ToolOutputErrorText, CoverageEntry>,
  },
  {
    id: "tool-output-error-json",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/11/output/type","expected":"error-json"},
      "value": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/11/output/value","presence":"present"},
      "providerOptions": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/11/output/providerOptions","presence":"present"},
    } satisfies Record<keyof ToolOutputErrorJSON, CoverageEntry>,
  },
  {
    id: "tool-output-content",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/12/output/type","expected":"content"},
      "value": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/12/output/value","presence":"present"},
    } satisfies Record<keyof ToolOutputContent, CoverageEntry>,
  },
  {
    id: "tool-output-content-text",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/12/output/value/0/type","expected":"text"},
      "text": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/12/output/value/0/text","presence":"present"},
      "providerOptions": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/12/output/value/0/providerOptions","presence":"present"},
    } satisfies Record<keyof ToolOutputContentText, CoverageEntry>,
  },
  {
    id: "tool-output-content-file",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"presence-losses","path":"/requests/0/body/prompt/3/content/1/output/value/0/type","expected":"file"},
      "data": {"scenario":"presence-losses","path":"/requests/0/body/prompt/3/content/1/output/value/0/data","presence":"present"},
      "mediaType": {"scenario":"presence-losses","path":"/requests/0/body/prompt/3/content/1/output/value/0/mediaType","expected":""},
      "filename": {"scenario":"presence-losses","path":"/requests/0/body/prompt/3/content/1/output/value/0/filename","expected":""},
      "providerOptions": {"scenario":"presence-losses","path":"/requests/0/body/prompt/3/content/1/output/value/0/providerOptions","expected":{}},
    } satisfies Record<keyof ToolOutputContentFile, CoverageEntry>,
  },
  {
    id: "tool-output-content-custom",
    source: "@ai-sdk/provider/src/language-model/v4/language-model-v4-prompt.ts",
    members: {
      "type": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/12/output/value/2/type","expected":"custom"},
      "providerOptions": {"scenario":"streaming-prompt-tools","path":"/requests/0/body/prompt/2/content/12/output/value/2/providerOptions","presence":"present"},
    } satisfies Record<keyof ToolOutputContentCustom, CoverageEntry>,
  },
  {
    id: "gateway-transform",
    source: "@ai-sdk/gateway/src/gateway-language-model.ts and @ai-sdk/provider-utils request helpers",
    members: {
      "abort-signal-excluded": {"exclusion":"abortSignal is removed from the JSON body and passed to Fetch."},
      "call-header-body": {"expected":"call-value","path":"/requests/0/body/headers/x-call","scenario":"header-call"},
      "call-header-outer": {"expected":"call-value","path":"/requests/0/headers/x-call","scenario":"header-call"},
      "case-variant-content-type-exclusion": {"expected":"unsupported-reserved-collision","path":"/requests/0/envelope","scenario":"header-case-content-type"},
      "case-variant-last-value": {"expected":"call","path":"/requests/0/headers/x-case","scenario":"header-case-custom"},
      "case-variant-protocol-last-value": {"expected":"4","path":"/requests/0/headers/ai-language-model-specification-version","scenario":"header-case-protocol"},
      "exact-content-type-exclusion": {"expected":"unsupported-reserved-collision","path":"/requests/0/envelope","scenario":"header-exact-content-type"},
      "exact-protocol-replacement": {"expected":"4","path":"/requests/0/headers/ai-language-model-specification-version","scenario":"header-exact-protocol"},
      "language-model-relative-url": {"expected":"/language-model","path":"/requests/0/path","scenario":"unary-settings"},
      "observability-last-value": {"expected":"deployment-evidence","path":"/requests/0/headers/ai-o11y-deployment-id","scenario":"header-observability"},
      "same-key-replacement": {"expected":"call","path":"/requests/0/headers/x-exact","scenario":"header-exact-custom"},
      "uint8array-base64": {"expected":"AAEC/w==","path":"/requests/0/body/prompt/1/content/1/data/data","scenario":"streaming-prompt-tools"},
      "call-trace-body": {"scenario":"header-call","path":"/requests/0/body/headers/traceparent","expected":"00-trace-parent"},
      "call-trace-outer": {"scenario":"header-call","path":"/requests/0/headers/traceparent","expected":"00-trace-parent"},
      "observability-body": {"scenario":"header-observability","path":"/requests/0/body/headers/ai-o11y-deployment-id","expected":"caller-observability"},
    } satisfies Record<GatewayTransform, CoverageEntry>,
  },
] as const satisfies readonly CoverageCategory[];
