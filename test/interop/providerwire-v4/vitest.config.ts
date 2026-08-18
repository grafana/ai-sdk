import { resolve } from "node:path";
import { defineConfig } from "vitest/config";

export default defineConfig({
  root: resolve(import.meta.dirname, ".."),
  test: {
    environment: "node",
    include: ["providerwire-v4/**/*.test.ts"],
    testTimeout: 30_000,
    hookTimeout: 30_000,
  },
});
