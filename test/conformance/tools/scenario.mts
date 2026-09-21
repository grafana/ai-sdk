import { existsSync } from "node:fs";
import { join } from "node:path";
import type { LanguageModelV4, LanguageModelV4CallOptions, LanguageModelV4GenerateResult } from "@ai-sdk/provider";
import { convertToModelMessages, streamText, stepCountIs, type LanguageModelUsage } from "ai";
import { buildMessages, buildOutput, buildStreamTextOptions, buildTools, createSourceIdNormalizer, loadConfig, mockId, unsupportedGenerateFields, type TestCase } from "./common.mts";

export interface ScenarioResult {
  chunks: unknown[];
  usage: unknown[];
  object: unknown;
  generate?: unknown;
  error?: string;
  outputError?: string;
}

export function generateResultSnapshot(result: LanguageModelV4GenerateResult) {
  const bedrockMetadata = result.providerMetadata?.bedrock;
  return {
    content: result.content,
    finishReason: result.finishReason,
    usage: result.usage,
    ...(bedrockMetadata ? { providerMetadata: { bedrock: bedrockMetadata } } : {}),
    ...(result.warnings?.length ? { warnings: result.warnings } : {}),
  };
}

function toProviderUsage(usage: LanguageModelUsage) {
  return {
    inputTokens: {
      total: usage.inputTokens,
      noCache: usage.inputTokenDetails.noCacheTokens,
      cacheRead: usage.inputTokenDetails.cacheReadTokens,
      cacheWrite: usage.inputTokenDetails.cacheWriteTokens,
    },
    outputTokens: {
      total: usage.outputTokens,
      text: usage.outputTokenDetails.textTokens,
      reasoning: usage.outputTokenDetails.reasoningTokens,
    },
    raw: usage.raw,
  };
}

export function normalizeOpenAIApprovalToolCallIds(providerName: string, chunks: unknown[]): unknown[] {
  if (providerName !== "openai") return chunks;
  const mcpToolCallIds = new Set<string>();
  for (const chunk of chunks) {
    if (typeof chunk === "object" && chunk !== null && "type" in chunk && chunk.type === "tool-input-available" &&
      "toolName" in chunk && typeof chunk.toolName === "string" && chunk.toolName.startsWith("mcp.") &&
      "toolCallId" in chunk && typeof chunk.toolCallId === "string") mcpToolCallIds.add(chunk.toolCallId);
  }
  const replacements = new Map<string, string>();
  for (const chunk of chunks) {
    if (typeof chunk === "object" && chunk !== null && "type" in chunk && chunk.type === "tool-approval-request" &&
      "toolCallId" in chunk && typeof chunk.toolCallId === "string" && mcpToolCallIds.has(chunk.toolCallId)) {
      replacements.set(chunk.toolCallId, `src-${replacements.size}`);
    }
  }
  return chunks.map(chunk => {
    if (typeof chunk !== "object" || chunk === null || !("toolCallId" in chunk) || typeof chunk.toolCallId !== "string") return chunk;
    const toolCallId = replacements.get(chunk.toolCallId);
    return toolCallId ? { ...chunk, toolCallId } : chunk;
  });
}

export async function executeScenario(tc: TestCase, model: LanguageModelV4, signal?: AbortSignal): Promise<ScenarioResult> {
  const cfg = loadConfig(tc.dir);
  const capture: ScenarioResult = { chunks: [], usage: [], object: null };
  if (cfg.operation === "generate") {
    if (tc.provider !== "bedrock") throw new Error("operation: generate is currently supported only for Bedrock");
    const unsupported = unsupportedGenerateFields(cfg);
    if (unsupported.length) throw new Error(`operation: generate does not support: ${unsupported.join(", ")}`);
    const prompt = cfg.messages
      ? [...(cfg.system ? [{ role: "system", content: cfg.system }] : []), ...(buildMessages(cfg, cfg.prompt ?? "test") ?? [])]
      : [...(cfg.system ? [{ role: "system", content: cfg.system }] : []), { role: "user", content: [{ type: "text", text: cfg.prompt ?? "test" }] }];
    const responseFormat = cfg.responseFormat ? {
      type: "json" as const,
      schema: cfg.responseFormat.schema,
      ...(cfg.responseFormat.name ? { name: cfg.responseFormat.name } : {}),
      ...(cfg.responseFormat.description ? { description: cfg.responseFormat.description } : {}),
    } : undefined;
    try {
      capture.generate = generateResultSnapshot(await model.doGenerate({
        prompt: prompt as LanguageModelV4CallOptions["prompt"],
        ...(responseFormat ? { responseFormat } : {}),
        ...(cfg.providerOptions ? { providerOptions: cfg.providerOptions as LanguageModelV4CallOptions["providerOptions"] } : {}),
        ...(cfg.headers ? { headers: cfg.headers } : {}),
        ...(signal ? { abortSignal: signal } : {}),
      }));
    } catch (error) {
      capture.error = String(error);
    }
    return capture;
  }
  const tools = buildTools(cfg.tools, cfg.providerTools);
  const prompt = cfg.prompt ?? "test";
  const messages = cfg.uiMessages ? await convertToModelMessages(cfg.uiMessages, { tools }) : buildMessages(cfg, prompt);
  const output = buildOutput(cfg);
  const options = buildStreamTextOptions(cfg, {
    model, messages, prompt, tools, output,
    stopWhen: stepCountIs(cfg.stopWhenStepCount ?? 1), generateId: mockId("id"),
  });
  const result = streamText({
    ...options,
    ...(signal ? { abortSignal: signal } : {}),
    onError: ({ error }) => { capture.error = String(error); },
  } as Parameters<typeof streamText>[0]);
  const normalize = tc.provider === "openai" ? createSourceIdNormalizer() : (chunk: unknown) => chunk;
  const outputPromise = output ? Promise.resolve(result.output).then(value => { capture.object = value; }, error => { capture.outputError = String(error); }) : undefined;
  const usagePromise = existsSync(join(tc.dir, "expected-usage.json"))
    ? result.steps.then(steps => { capture.usage = steps.map(step => toProviderUsage(step.usage)); }, error => { capture.error ??= String(error); })
    : undefined;
  try {
    for await (const chunk of result.toUIMessageStream(cfg.streamOptions ?? {})) capture.chunks.push(normalize(chunk));
  } catch (error) {
    capture.error = String(error);
  }
  await Promise.all([outputPromise, usagePromise]);
  capture.chunks = normalizeOpenAIApprovalToolCallIds(tc.provider, capture.chunks);
  return capture;
}
