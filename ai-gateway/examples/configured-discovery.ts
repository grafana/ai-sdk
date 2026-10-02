export interface ConfiguredCandidate {
  providerInstance: string;
  provider: string;
  providerModelId: string;
}

export interface ConfiguredRoute {
  aliases: string[];
  primary: ConfiguredCandidate;
  fallbacks: ConfiguredCandidate[];
}

export interface ConfiguredModel {
  id: string;
  name: string;
  description?: string | null;
  specification: { specificationVersion: string; provider: string; modelId: string };
  gateway?: ConfiguredRoute;
}

const maxDocumentBytes = 4_194_304;
const initialReadBufferBytes = 4096;

function invalid(): never {
  throw new Error("configured discovery: invalid catalog");
}

function object(value: unknown): Record<string, unknown> {
  if (value === null || typeof value !== "object" || Array.isArray(value)) invalid();
  return value as Record<string, unknown>;
}

function text(value: unknown): string {
  if (typeof value !== "string") invalid();
  return value;
}

function collection(value: unknown): unknown[] {
  if (!Array.isArray(value)) invalid();
  return value;
}

function candidate(value: unknown): ConfiguredCandidate {
  const source = object(value);
  return { providerInstance: text(source.providerInstance), provider: text(source.provider), providerModelId: text(source.providerModelId) };
}

function route(value: unknown): ConfiguredRoute {
  const source = object(value);
  return { aliases: collection(source.aliases).map(text), primary: candidate(source.primary), fallbacks: collection(source.fallbacks).map(candidate) };
}

function catalog(value: unknown): { models: ConfiguredModel[] } {
  const rows = collection(object(value).models);
  const models: ConfiguredModel[] = rows.map(value => {
    const source = object(value);
    const specification = object(source.specification);
    const model: ConfiguredModel = { id: text(source.id), name: text(source.name), specification: { specificationVersion: text(specification.specificationVersion), provider: text(specification.provider), modelId: text(specification.modelId) } };
    if (Object.hasOwn(source, "description")) model.description = source.description === null ? null : text(source.description);
    if (source.gateway != null) model.gateway = route(source.gateway);
    return model;
  });
  return { models };
}

export async function fetchConfiguredModels({ baseURL, headers, fetch: fetcher = globalThis.fetch, signal, maxBytes = maxDocumentBytes }: {
  baseURL: string;
  headers: HeadersInit;
  fetch?: typeof globalThis.fetch;
  signal?: AbortSignal;
  maxBytes?: number;
}): Promise<{ models: ConfiguredModel[] }> {
  let url: URL;
  try { url = new URL(baseURL); } catch { throw new Error("configured discovery: invalid base URL"); }
  if (!["http:", "https:"].includes(url.protocol) || url.username || url.password || url.href.includes("?") || url.href.includes("#")) throw new Error("configured discovery: invalid base URL");
  if (!Number.isSafeInteger(maxBytes) || maxBytes <= 0 || maxBytes > maxDocumentBytes) throw new Error("configured discovery: invalid byte limit");
  signal?.throwIfAborted();
  let response: Response;
  try {
    response = await fetcher(url.href.replace(/\/$/, "") + "/config", { method: "GET", headers, signal, redirect: "error", credentials: "omit" });
  } catch {
    signal?.throwIfAborted();
    throw new Error("configured discovery: request failed");
  }
  const reader = response.body?.getReader();
  if (!reader) throw new Error("configured discovery: missing body");
  let failed = false;
  let canceled: Promise<void> | undefined;
  const onAbort = () => { canceled = reader.cancel(signal?.reason); };
  signal?.addEventListener("abort", onAbort, { once: true });
  try {
    signal?.throwIfAborted();
    if (!response.ok || response.redirected) throw new Error("configured discovery: request rejected");
    const media = response.headers.get("content-type")?.split(";", 1)[0]?.trim().toLowerCase();
    if (media !== "application/json" && !/^application\/[a-z0-9!#$&^_.+-]+\+json$/.test(media ?? "")) throw new Error("configured discovery: expected JSON");
    let bytes = new Uint8Array(Math.min(maxBytes, initialReadBufferBytes));
    let size = 0;
    for (;;) {
      let result: ReadableStreamReadResult<Uint8Array>;
      try { result = await reader.read(); } catch { signal?.throwIfAborted(); throw new Error("configured discovery: read failed"); }
      signal?.throwIfAborted();
      if (result.done) break;
      const nextSize = size + result.value.byteLength;
      if (nextSize > maxBytes) throw new Error("configured discovery: byte limit exceeded");
      if (nextSize > bytes.byteLength) {
        const grown = new Uint8Array(Math.min(maxBytes, Math.max(nextSize, bytes.byteLength * 2)));
        grown.set(bytes);
        bytes = grown;
      }
      bytes.set(result.value, size);
      size = nextSize;
    }
    let value: unknown;
    try { value = JSON.parse(new TextDecoder("utf-8", { fatal: true, ignoreBOM: true }).decode(bytes.subarray(0, size))); } catch { invalid(); }
    return catalog(value);
  } catch (error) {
    failed = true;
    throw error;
  } finally {
    signal?.removeEventListener("abort", onAbort);
    try { await (canceled ?? reader.cancel()); }
    catch { if (!failed) throw new Error("configured discovery: cleanup failed"); }
    finally { reader.releaseLock(); }
  }
}
