// vitest/config includes types for the test options.
import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// task dev proxies /api to the Go server.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    // Keep this alias aligned with tsconfig.json for shadcn imports.
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  build: {
    // Preserve tracked .gitkeep so Go builds work without a built SPA; scripts/build removes old output without deleting the embed placeholder.
    outDir: "dist",
    emptyOutDir: false,
  },
  server: {
    port: 5173,
    // Ignore test build output so development reloads do not interrupt playback or API requests.
    watch: { ignored: ["**/.vite-build-check/**"] },
    proxy: {
      "/api": {
        target: process.env.MDM_API_TARGET ?? "http://localhost:8080",
        // Preserve the browser-facing Host so the API's Origin check sees
        // the same authority on both sides during development.
        changeOrigin: false,
      },
    },
  },
  // Use jsdom for DOM tests without Tailwind transforms; contrast tests read CSS directly.
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./vitest.setup.ts"],
    css: false,
    // vmThreads reuses workers while giving each file a fresh VM and DOM, avoiding repeated dependency loading without sharing module state.
    pool: "vmThreads",
    // Measure only with --coverage, excluding generated bindings and test helpers.
    coverage: {
      provider: "v8",
      include: ["src/**/*.{ts,tsx}"],
      exclude: ["src/api/gen/**", "src/testing/**", "src/**/*.test.{ts,tsx}"],
      reporter: ["lcovonly", "text-summary"],
    },
  },
});
