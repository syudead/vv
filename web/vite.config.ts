// defineConfig は vitest/config から取る。Vite のものは test の項を型として知らない。
import { defineConfig } from "vitest/config";
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
  // 単体テスト（make test-web が npm run test 経由で vitest run を呼ぶ）。
  // DOM を伴う検証があるので環境は jsdom。css: false で Tailwind の変換を挟まない
  // ── 見た目の検査（対比）は CSS をファイルとして読むので、描画する必要が無い。
  // passWithNoTests は実行基盤だけを先に置く段階（tasks.md Phase 1 の Checkpoint）で
  // make test-web を成功で終わらせるためにある。テストが揃ったら外してよい。
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./vitest.setup.ts"],
    css: false,
    passWithNoTests: true,
  },
});
