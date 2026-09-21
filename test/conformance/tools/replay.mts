import { createServer } from "node:http";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { normalizeRequestSnapshot, type RequestSnapshot, type TestCase } from "./common.mts";

export function replayAdapter(provider: string) {
  switch (provider) {
    case "anthropic": return { framing: "sse", basePath: "" } as const;
    case "openai":
    case "openai-compatible": return { framing: "sse", basePath: "/v1" } as const;
    case "bedrock": return { framing: "bedrock", basePath: "" } as const;
    default: throw new Error(`missing replay/configuration adapter for provider: ${provider}`);
  }
}

const CRC_TABLE = Array.from({ length: 256 }, (_, i) => {
  let c = i;
  for (let j = 0; j < 8; j++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
  return c >>> 0;
});
function crc32(buffer: Uint8Array) {
  let c = 0xffffffff;
  for (const value of buffer) c = CRC_TABLE[(c ^ value) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}
function header(name: string, value: string) {
  const length = Buffer.alloc(2);
  length.writeUInt16BE(Buffer.byteLength(value));
  return Buffer.concat([Buffer.from([name.length]), Buffer.from(name), Buffer.from([7]), length, Buffer.from(value)]);
}
function bedrockFrame(line: string) {
  const event = JSON.parse(line) as Record<string, unknown>;
  const keys = Object.keys(event);
  if (keys.length !== 1) throw new Error("Bedrock fixture event must contain one key");
  const type = keys[0];
  const headers = Buffer.concat([header(":message-type", "event"), header(":event-type", type), header(":content-type", "application/json")]);
  const payload = Buffer.from(JSON.stringify(event[type]));
  const prelude = Buffer.alloc(12);
  prelude.writeUInt32BE(16 + headers.length + payload.length, 0);
  prelude.writeUInt32BE(headers.length, 4);
  prelude.writeUInt32BE(crc32(prelude.subarray(0, 8)), 8);
  const body = Buffer.concat([prelude, headers, payload]);
  const checksum = Buffer.alloc(4);
  checksum.writeUInt32BE(crc32(body));
  return Buffer.concat([body, checksum]);
}

export function fixtureResponse(fixture: string, provider: string): Buffer {
  const lines = fixture.split("\n").filter(Boolean);
  if (replayAdapter(provider).framing === "bedrock") return Buffer.concat(lines.map(bedrockFrame));
  return Buffer.from(lines.map(line => `event: ${JSON.parse(line).type ?? "unknown"}\ndata: ${line}\n\n`).join(""));
}

export async function startReplay(tc: TestCase, host = "127.0.0.1") {
  const adapter = replayAdapter(tc.provider);
  const unary = existsSync(join(tc.dir, "input.response.json"));
  const responses: Buffer[] = [];
  if (unary) responses.push(readFileSync(join(tc.dir, "input.response.json")));
  else if (existsSync(join(tc.dir, "input.chunks.txt"))) responses.push(fixtureResponse(readFileSync(join(tc.dir, "input.chunks.txt"), "utf8"), tc.provider));
  else for (let i = 1; existsSync(join(tc.dir, `input-${i}.chunks.txt`)); i++) {
    responses.push(fixtureResponse(readFileSync(join(tc.dir, `input-${i}.chunks.txt`), "utf8"), tc.provider));
  }
  if (!responses.length) throw new Error(`no fixtures: ${tc.name}`);
  let index = 0;
  const requests: RequestSnapshot[] = [];
  const errors: string[] = [];
  const server = createServer((req, res) => {
    const chunks: Buffer[] = [];
    req.on("error", error => { errors.push(String(error)); res.destroy(); });
    req.on("data", (chunk: Buffer) => chunks.push(chunk));
    req.on("end", () => {
      try {
        requests.push(normalizeRequestSnapshot(tc.provider, req, Buffer.concat(chunks).toString("utf8")));
      } catch (error) {
        errors.push(String(error));
        res.writeHead(400).end("Invalid request snapshot");
        return;
      }
      const response = responses[index++];
      if (!response) {
        res.writeHead(500).end("No more fixtures");
        return;
      }
      res.writeHead(200, {
        "Content-Type": unary ? "application/json" : adapter.framing === "bedrock" ? "application/vnd.amazon.eventstream" : "text/event-stream",
        "Cache-Control": "no-cache",
      });
      res.end(response);
    });
  });
  server.requestTimeout = 10_000;
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, host, () => { server.removeListener("error", reject); resolve(); });
  });
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("missing replay listener address");
  return {
    url: `http://${host}:${address.port}`, requests, errors,
    close: () => new Promise<void>((resolve, reject) => {
      server.close(error => error ? reject(error) : resolve());
      server.closeAllConnections();
    }),
  };
}
