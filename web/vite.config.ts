import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// 開発時は Go サーバー（既定 :8080）へ /api を中継する。make dev がこの構成で動く。
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    // Go 側が embed する出力先。web/dist は版管理にプレースホルダを置いてある。
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});
