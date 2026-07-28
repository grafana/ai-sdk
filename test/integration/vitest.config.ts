import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    globalSetup: ["./global-setup.ts"],
    testTimeout: 30_000,
    hookTimeout: 60_000,
  },
});
