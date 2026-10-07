import { readFileSync } from "node:fs";
import { createServer, request as httpRequest } from "node:http";
import { resolve } from "node:path";
import { createSecureContext, TLSSocket } from "node:tls";

export const nativeProxyCertificate = resolve(import.meta.dirname, "fixtures/native-proxy/cert.pem");

export async function startNativeProviderProxy(target: string, authority: "api.anthropic.com:443" | "api.openai.com:443" = "api.anthropic.com:443") {
  const context = createSecureContext({
    cert: readFileSync(nativeProxyCertificate),
    key: readFileSync(resolve(import.meta.dirname, "fixtures/native-proxy/key.pem")),
  });
  const sockets = new Set<TLSSocket>();
  const forwarded = createServer((request, response) => {
    const upstream = httpRequest(new URL(request.url ?? "/", target), {
      method: request.method, headers: request.headers,
    }, result => {
      response.writeHead(result.statusCode!, result.headers);
      result.pipe(response);
    });
    upstream.on("error", () => response.destroy());
    response.on("close", () => upstream.destroy());
    request.pipe(upstream);
  });
  const proxy = createServer((_request, response) => { response.writeHead(405).end(); });
  proxy.on("connect", (request, socket, head) => {
    if (request.url !== authority) {
      socket.end("HTTP/1.1 403 Forbidden\r\n\r\n");
      return;
    }
    socket.write("HTTP/1.1 200 Connection Established\r\n\r\n");
    if (head.length) socket.unshift(head);
    const secure = new TLSSocket(socket, { isServer: true, secureContext: context });
    sockets.add(secure);
    secure.on("error", () => secure.destroy());
    secure.on("close", () => sockets.delete(secure));
    forwarded.emit("connection", secure);
  });
  await new Promise<void>(resolve => proxy.listen(0, "127.0.0.1", resolve));
  const address = proxy.address();
  if (!address || typeof address === "string") throw new Error("native proxy did not bind TCP");
  return {
    url: `http://127.0.0.1:${address.port}`,
    async stop() {
      for (const socket of sockets) socket.destroy();
      forwarded.closeAllConnections();
      proxy.closeAllConnections();
      await new Promise<void>(resolve => proxy.close(() => resolve()));
    },
  };
}
