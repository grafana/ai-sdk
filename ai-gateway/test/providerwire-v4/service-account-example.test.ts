import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { serviceAccountGateway } from "./service-account-example";

describe("service account client example", () => {
  it("targets the plugin route prefix with one bearer credential", async () => {
    const requests: Array<{ url: string; headers: Headers }> = [];
    const gateway = serviceAccountGateway({
      grafanaURL: "https://grafana.example.com/",
      namespace: "stacks-123",
      token: "glsa_dummy-service-account",
      fetch: async (input, init) => {
        requests.push({ url: String(input), headers: new Headers(init?.headers) });
        return new Response(JSON.stringify({ models: [] }), { headers: { "Content-Type": "application/json" } });
      },
    });
    assert.deepEqual((await gateway.getAvailableModels()).models, []);
    assert.equal(requests.length, 1);
    assert.equal(requests[0]!.url, "https://grafana.example.com/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123/config");
    assert.equal(requests[0]!.headers.get("authorization"), "Bearer glsa_dummy-service-account");
    assert.equal(requests[0]!.headers.get("x-access-token"), null);
  });
});
