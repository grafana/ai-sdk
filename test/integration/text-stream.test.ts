import { describe, it, expect } from "vitest";
import { fetchScenario } from "./helpers.js";

describe("Text stream", () => {
  it("text-stream scenario produces valid accumulated JSON", async () => {
    const res = await fetchScenario("text-stream");

    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toBe(
      "text/plain; charset=utf-8",
    );

    const body = await res.text();
    expect(body.length).toBeGreaterThan(0);

    const parsed = JSON.parse(body);
    expect(parsed).toEqual({
      name: "Alice",
      age: 30,
      active: true,
    });
  });
});
