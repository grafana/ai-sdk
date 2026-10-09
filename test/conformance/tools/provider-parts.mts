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

export function normalizeProviderPart(part: Part): Part {
  const { timestamp: _timestamp, ...rest } = part;
  switch (rest.type) {
    case "stream-start":
      return { type: "stream-start", warnings: rest.warnings ?? [] };
    case "response-metadata":
      return {
        type: "response-metadata",
        ...(rest.id != null ? { id: rest.id } : {}),
        ...(rest.modelId != null ? { modelId: rest.modelId } : {}),
      };
    case "error":
      return { type: "error" };
    case "tool-result":
      return { ...JSON.parse(JSON.stringify(rest)), isError: rest.isError === true };
    default:
      return JSON.parse(JSON.stringify(rest));
  }
}

export function normalizeCall(parts: Part[]): Part[] {
  const normalized = parts.map(normalizeProviderPart);
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

export function serializeProviderParts(calls: Part[][]): string {
  return calls.map(parts => JSON.stringify({ parts: normalizeCall(parts) })).join("\n") + "\n";
}

export function writeProviderParts(path: string, calls: Part[][]): void {
  writeFileSync(path, serializeProviderParts(calls));
}
