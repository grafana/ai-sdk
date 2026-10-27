import { createServer, type IncomingMessage, type ServerResponse } from "node:http";

export async function withReplayServer<T>(
  body: string,
  headers: Record<string, string>,
  run: (port: number) => Promise<T>,
  onRequest?: (req: IncomingMessage, body: string) => void,
): Promise<T> {
  const server = createServer((req: IncomingMessage, res: ServerResponse) => {
    const chunks: Buffer[] = [];
    req.on("data", (chunk: Buffer) => chunks.push(chunk));
    req.on("end", () => {
      onRequest?.(req, Buffer.concat(chunks).toString("utf8"));
      res.writeHead(200, headers);
      res.end(body);
    });
  });
  const port = await new Promise<number>(resolvePort => {
    server.listen(0, "127.0.0.1", () => resolvePort((server.address() as { port: number }).port));
  });
  try {
    return await run(port);
  } finally {
    await new Promise<void>(done => server.close(() => done()));
  }
}
