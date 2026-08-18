import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "node",
    include: ["interop.test.ts"],
    globalSetup: ["./global-setup.ts"],
    testTimeout: 30_000,
    hookTimeout: 60_000,
  },
});
