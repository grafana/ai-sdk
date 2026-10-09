import { writeFileSync } from "node:fs";

export const PROVIDER_PARTS_FILE = "expected-provider-parts.jsonl";

export const PROVIDER_PARTS_PROVIDERS = new Set(["anthropic", "openai", "bedrock", "openai-compatible"]);

type Part = Record<string, unknown>;

export type ProviderPartsRecorder = {
  calls: Part[][];
};

export function recordProviderParts<T extends object>(model: T): { model: T; recorder: ProviderPartsRecorder } {
  const recorder: ProviderPartsRecorder = { calls: [] };
  const original = (model as any).doStream.bind(model);
  const wrapped = Object.create(model) as T;
  (wrapped as any).doStream = async (options: Record<string, unknown>) => {
    const result = await original({ ...options, includeRawChunks: true });
    const parts: Part[] = [];
    recorder.calls.push(parts);
    const stream = (result.stream as ReadableStream<Part>).pipeThrough(
      new TransformStream<Part, Part>({
        transform(part, controller) {
          parts.push(part);
          controller.enqueue(part);
        },
      }),
    );
    return { ...result, stream };
  };
  return { model: wrapped, recorder };
}

const VOLATILE_TIMESTAMP_PROVIDERS = new Set(["bedrock"]);

export function normalizeProviderPart(part: Part, providerName: string): Part {
  const plain = JSON.parse(JSON.stringify(part));
  switch (plain.type) {
    case "response-metadata": {
      const { id, modelId, timestamp } = plain;
      return {
        type: "response-metadata",
        ...(id != null ? { id } : {}),
        ...(modelId != null ? { modelId } : {}),
        ...(timestamp != null && !VOLATILE_TIMESTAMP_PROVIDERS.has(providerName) ? { timestamp } : {}),
      };
    }
    case "error": {
      const error = part.error as any;
      return typeof error?.statusCode === "number"
        ? {
            type: "error",
            error: {
              message: String(error.message),
              statusCode: error.statusCode,
              isRetryable: error.isRetryable === true,
            },
          }
        : { type: "error" };
    }
    case "tool-result":
      return { ...plain, isError: plain.isError === true };
    default:
      return plain;
  }
}

export function normalizeCall(parts: Part[], providerName: string): Part[] {
  const normalized = parts.map(part => normalizeProviderPart(part, providerName));
  const generatedToolCallIds = new Map<string, string>();
  for (const part of normalized) {
    if (
      part.type === "tool-call" &&
      typeof part.toolName === "string" &&
      part.toolName.startsWith("mcp.") &&
      typeof part.toolCallId === "string" &&
      !generatedToolCallIds.has(part.toolCallId)
    ) {
      generatedToolCallIds.set(part.toolCallId, `mcp-${generatedToolCallIds.size}`);
    }
  }
  let sourceCount = 0;
  return normalized.map(part => {
    if (part.type === "source") return { ...part, id: `src-${sourceCount++}` };
    const generated = typeof part.toolCallId === "string" ? generatedToolCallIds.get(part.toolCallId) : undefined;
    return generated ? { ...part, toolCallId: generated } : part;
  });
}

export function serializeProviderParts(calls: Part[][], providerName: string): string {
  return calls.map(parts => JSON.stringify({ parts: normalizeCall(parts, providerName) })).join("\n") + "\n";
}

export function writeProviderParts(path: string, calls: Part[][], providerName: string): void {
  writeFileSync(path, serializeProviderParts(calls, providerName));
}
