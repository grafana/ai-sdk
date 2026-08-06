import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { createGateway } from "@ai-sdk/gateway";

const URL_FILE = resolve(import.meta.dirname, ".test-server-url");

export type ProviderWireEndpoint = "legacy" | "strict";

let cachedUrls: Record<ProviderWireEndpoint, string> | undefined;

/** getGatewayBaseURL returns one local Go provider-wire gateway base URL. */
export function getGatewayBaseURL(endpoint: ProviderWireEndpoint = "legacy"): string {
  if (!cachedUrls) {
    try {
      cachedUrls = JSON.parse(readFileSync(URL_FILE, "utf-8")) as Record<ProviderWireEndpoint, string>;
    } catch {
      throw new Error(
        "Interop test server URL file not found. Is the global setup running correctly?",
      );
    }
  }
  return cachedUrls[endpoint];
}

/** newGateway builds an upstream gateway provider pointed at one Go handler. */
export function newGateway(endpoint: ProviderWireEndpoint = "legacy") {
  return createGateway({
    apiKey: "interop-not-a-real-key",
    baseURL: getGatewayBaseURL(endpoint),
  });
}
