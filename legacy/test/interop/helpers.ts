import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { createGateway } from "@ai-sdk/gateway";

const URL_FILE = resolve(import.meta.dirname, ".test-server-url");

let cachedUrl: string | undefined;

/** getGatewayBaseURL returns the local Go provider-wire gateway base URL. */
export function getGatewayBaseURL(): string {
  if (cachedUrl) return cachedUrl;
  try {
    cachedUrl = readFileSync(URL_FILE, "utf-8").trim();
  } catch {
    throw new Error(
      "Interop test server URL file not found. Is the global setup running correctly?",
    );
  }
  return cachedUrl;
}

/** newGateway builds an upstream @ai-sdk/gateway provider pointed at the Go server. */
export function newGateway() {
  return createGateway({
    apiKey: "interop-not-a-real-key",
    baseURL: getGatewayBaseURL(),
  });
}
