import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const URL_FILE = resolve(import.meta.dirname, ".test-server-url");

let cachedUrl: string | undefined;

export function getServerUrl(): string {
  if (cachedUrl) return cachedUrl;
  try {
    cachedUrl = readFileSync(URL_FILE, "utf-8").trim();
  } catch {
    throw new Error(
      "Test server URL file not found. Is the global setup running correctly?",
    );
  }
  return cachedUrl;
}

export async function fetchScenario(
  name: string,
  options?: RequestInit,
): Promise<Response> {
  const url = getServerUrl();
  return fetch(`${url}/scenario/${name}`, {
    method: "POST",
    ...options,
  });
}
