import {
  createGateway,
  GatewayInvalidRequestError,
  GatewayModelNotFoundError,
} from "@ai-sdk/gateway";
import { describe, expect, it } from "vitest";
import { getServerUrl } from "./helpers.js";

const POLL_INTERVAL_MS = 10;
const POLL_TIMEOUT_MS = 2_000;

function model(modelID: string) {
  return createGateway({
    apiKey: "integration-test-key",
    baseURL: `${getServerUrl()}/providerwire-v4`,
  })(modelID);
}

async function stats(): Promise<{ successCalls: number; blockingCalls: number; cancellations: number }> {
  const response = await fetch(`${getServerUrl()}/providerwire-v4/stats`);
  expect(response.ok).toBe(true);
  return await response.json() as { successCalls: number; blockingCalls: number; cancellations: number };
}

async function waitFor(load: () => Promise<boolean>): Promise<void> {
  const deadline = Date.now() + POLL_TIMEOUT_MS;
  while (Date.now() < deadline) {
    if (await load()) return;
    await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
  }
  throw new Error(`condition not met within ${POLL_TIMEOUT_MS}ms`);
}

describe("ProviderWire V4 unary runtime", () => {
  it("consumes the minimal production response through the pinned Gateway client", async () => {
    const result = await model("success").doGenerate({
      prompt: [{ role: "system", content: "hello" }],
    });

    expect(result.content).toEqual([{ type: "text", text: "hello from Go" }]);
    expect(result.finishReason).toEqual({ unified: "stop", raw: "test-stop" });
    expect(result.usage).toEqual({
      inputTokens: { total: 2, noCache: 1, cacheRead: 1, cacheWrite: 0 },
      outputTokens: { total: 1, text: 1, reasoning: 0 },
    });
    expect(result.warnings).toEqual([]);
    expect(result.response?.body).toEqual({
      content: [{ type: "text", text: "hello from Go" }],
      finishReason: { unified: "stop", raw: "test-stop" },
      usage: {
        inputTokens: { total: 2, noCache: 1, cacheRead: 1, cacheWrite: 0 },
        outputTokens: { total: 1, text: 1, reasoning: 0 },
      },
    });
    expect((await stats()).successCalls).toBeGreaterThan(0);
  });

  it("maps representative runtime failures to pinned client classes", async () => {
    await expect(model("success").doGenerate({
      prompt: [],
      headers: { "x-body-header": "unsupported" },
    })).rejects.toSatisfy((error: unknown) => GatewayInvalidRequestError.isInstance(error));

    await expect(model("missing").doGenerate({ prompt: [] }))
      .rejects.toSatisfy((error: unknown) => GatewayModelNotFoundError.isInstance(error));
  });

  it("propagates cancellation to the production handler", async () => {
    const initial = await stats();
    const controller = new AbortController();
    const pending = model("blocking").doGenerate({ prompt: [], abortSignal: controller.signal });
    await waitFor(async () => (await stats()).blockingCalls > initial.blockingCalls);

    controller.abort();
    await expect(pending).rejects.toBeDefined();
    await waitFor(async () => (await stats()).cancellations > initial.cancellations);
  });
});
