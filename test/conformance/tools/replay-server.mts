import { createServer, type IncomingMessage, type ServerResponse } from "node:http";

export async function withReplayServer<T>(
  body: string,
  headers: Record<string, string>,
  run: (port: number) => Promise<T>,
): Promise<T> {
  const server = createServer((req: IncomingMessage, res: ServerResponse) => {
    req.resume();
    req.on("end", () => {
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
