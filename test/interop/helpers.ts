import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { createGateway } from "@ai-sdk/gateway";

const URL_FILE = resolve(import.meta.dirname, ".test-server-url");
const V4_URL_FILE = resolve(import.meta.dirname, ".test-server-v4-url");

let cachedUrl: string | undefined;
let cachedV4Url: string | undefined;

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

export function getV4GatewayBaseURL(): string {
  if (cachedV4Url) return cachedV4Url;
  try {
    cachedV4Url = readFileSync(V4_URL_FILE, "utf-8").trim();
  } catch {
    throw new Error(
      "ProviderWire V4 interop test server URL file not found. Is the global setup running correctly?",
    );
  }
  return cachedV4Url;
}

export function newV4Gateway() {
  return createGateway({
    apiKey: "interop-v4-not-a-real-key",
    baseURL: getV4GatewayBaseURL(),
  });
}
