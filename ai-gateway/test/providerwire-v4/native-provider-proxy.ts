import { readFileSync } from "node:fs";
import { createServer, request as httpRequest } from "node:http";
import { resolve } from "node:path";
import { createSecureContext, TLSSocket } from "node:tls";

export const nativeProxyCertificate = resolve(import.meta.dirname, "fixtures/native-proxy/cert.pem");

const nativeProxyKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEA1t2JLr+ylfCjaP3en+NBOYyR6pI9LfoDWdpYJxHaeoK3v42z
+OLRvqe8RcJ/LrUTxHALRdWPEceVYUJKL0Z8QMaPLk22fY8/06YmDhTD4L4jxddA
54EGlR3zb1gAGiMfFEE2nxi4IBKIYE0+qarPXTEePtNEA+VusNkuiD9cug0BM3HB
VjrOEBWFfjyt3yx6Qh/f5NVgzQjzYtVG+OcvDPnyZktXvhQ9cJuu3N9wZO985Rlm
P/2pqxVP051qh+Qv1wePd2IOpXIFCEEwdEtHs5n4Zvg5lZpiWBlN+z93YNdNkLhp
AREWVa9UIXLwudS1vS6phHAaLJG/EgwsqUwcIQIDAQABAoH/CMC0aDMdyiJxF/pk
F0PhiORVn6ZVaffFT78njwIrLWS4F4NAiHVHOqGanBhQOpbcpP9ZLRFheXyDUCnQ
29MDPMTZ67/Q+HECBArlONIyA/W67Qsg4t+n0rRlo/7xuPpHkvleQEF4Y0PLuCCR
/zLRiNXAijxaGzw9aq23XB6mXzbwr1cgmgHvSq8QA/xuhK0GMfaor90MgDBTgnMv
lL80YSJaeU3yhuTmLx1ITQM72W90QQLSDy2c5KcrthpDzN5MKYCTSB0bJvcHZ2VZ
g7O2be9QSWTuh0GCSR5oDAtWpUnqGAYYJF+V26vydIIfB/e+iZOo2DW5vt0BoWVF
s3U9AoGBANueAMMdECt0ejobddHFLb/y841O4RO878wH7M6K/q2xToJSoxBlFtgP
yk4ljnklwB8NeK55tFYtAS7WhAMHDtGM0a53f3yvsmZL5cpHT0/nua9cVZqtkcul
8btRNKxaWySWGXXrFk3J6m25bJZ3TNDKPY9JfhmwQoHmAi3lpWrHAoGBAPp2AeAW
Sw78pCGrOiBg/3BS+00jboM+3kdO3mmkKWIzyos7nfBO2mN1hs3Bm4Nm3kXoQ7YM
dHFNeEIgeKyEk9hIHMM8HzBBJ+LU/0HNB+2OZNbkN49FnnfeXLitAVRjzRiS7n7L
YCiP+sc9SNzMnvEYpiFrvW5KyQ37mKg6fhnXAoGBAJzJARxKLFgJkJTZM9StIwss
5AkWrgLMWJldcwbFHipcMYNCgZ1bveJD65a8oykD9VN155kP09nNyVFp3dbXfBHH
qY4XS5F1UTRMrOEq5YlTEjIKBicmELbFYnzq7WK6IuVMryKK0WJ5av4oaUhGJTXN
nAMmYXrvZZuc3CNuFhjTAoGACfaNxos8eyEjqk80ZbtWDfLPGldxevkSQIXrpQop
t0VWJkm906RfXZt8PE0aUZTS/Lbrkp9WNneddAv0oPA5LV5Y/o8ysmm1G3nbmZN9
YD7M1huH9kQPtLb8uz/ukJvTucmGgTa34YUwtaJDdr0RCYgwe53ckDmbW9oJTY/e
GksCgYEAlkcZbTymnqDBIIkY9/U1enKvVjCJX/5cQ63zvDkaWSASBbsyr71lp3ax
AZQyZzpO1m2j7ssH8rnVewhnqwAJ34q2IakKRIgtfelx+zsYbGLe1R2s8aJJXm0m
Ls6diS9KzfBJWfXD/xMY6j+zruhyaKYlUK/ffIFVbVYMUA4+Xck=
-----END RSA PRIVATE KEY-----`; // trufflehog:ignore

export async function startNativeProviderProxy(target: string, authority: "api.anthropic.com:443" | "api.openai.com:443" = "api.anthropic.com:443") {
  const context = createSecureContext({
    cert: readFileSync(nativeProxyCertificate),
    key: nativeProxyKey,
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
